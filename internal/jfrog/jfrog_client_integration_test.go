//go:build integration

package jfrog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestListDockerTags(t *testing.T) {
	const (
		repository = "docker-local"
		image      = "myteam/myapp"
	)
	wantTags := []string{"1.0.0", "1.1.0", "latest"}

	var gotAuthUser, gotAuthPass string
	var gotAuthOK bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, gotAuthPass, gotAuthOK = r.BasicAuth()

		wantPath := fmt.Sprintf("/artifactory/api/docker/%s/v2/%s/tags/list", repository, image)
		if r.URL.Path != wantPath {
			http.Error(w, fmt.Sprintf("unexpected path %q, want %q", r.URL.Path, wantPath), http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "want GET", http.StatusMethodNotAllowed)
			return
		}

		_ = json.NewEncoder(w).Encode(TagsResponse{
			Name: image,
			Tags: wantTags,
		})
	}))
	defer srv.Close()

	c := NewClient(Config{
		JFrogURL: srv.URL,
		Auth:     Auth{Username: "alice", Password: "s3cret"},
	})

	tags, err := c.ListDockerTags(context.Background(), repository, image)
	if err != nil {
		t.Fatalf("ListDockerTags: %v", err)
	}
	if !slices.Equal(tags, wantTags) {
		t.Errorf("tags = %v, want %v", tags, wantTags)
	}
	if !gotAuthOK || gotAuthUser != "alice" || gotAuthPass != "s3cret" {
		t.Errorf("basic auth = (%q, %q, ok=%v), want (alice, s3cret, ok=true)", gotAuthUser, gotAuthPass, gotAuthOK)
	}
}

func TestListDockerTags_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	_, err := c.ListDockerTags(context.Background(), "r", "i")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want mention of status 404", err)
	}
}

func TestXrayExportDetails(t *testing.T) {
	const (
		repository = "docker-local"
		image      = "myteam/myapp"
		version    = "1.2.3"
	)
	sbomJSON := []byte(`{"bomFormat":"CycloneDX","specVersion":"1.5"}`)

	var gotReq SBOMRequest
	var gotContentType string
	var gotAuthUser, gotAuthPass string
	var gotAuthOK bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, gotAuthPass, gotAuthOK = r.BasicAuth()
		gotContentType = r.Header.Get("Content-Type")

		if r.URL.Path != "/xray/api/v2/component/exportDetails" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Build a zip that contains a noise file and the JSON SBOM.
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		noise, _ := zw.Create("readme.txt")
		_, _ = noise.Write([]byte("ignore me"))
		sbom, _ := zw.Create("sbom.json")
		_, _ = sbom.Write(sbomJSON)
		if err := zw.Close(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = io.Copy(w, &buf)
	}))
	defer srv.Close()

	c := NewClient(Config{
		JFrogURL: srv.URL,
		Auth:     Auth{Username: "bob", Password: "hunter2"},
	})

	got, err := c.XrayExportDetails(context.Background(), repository, image, version)
	if err != nil {
		t.Fatalf("XrayExportDetails: %v", err)
	}
	if string(got) != string(sbomJSON) {
		t.Errorf("SBOM body = %q, want %q", got, sbomJSON)
	}

	wantReq := SBOMRequest{
		PackageType:     "docker",
		ComponentName:   fmt.Sprintf("%s:%s", image, version),
		Path:            fmt.Sprintf("%s/%s/%s/manifest.json", repository, image, version),
		CycloneDX:       true,
		CycloneDXFormat: "json",
	}
	if gotReq != wantReq {
		t.Errorf("SBOMRequest = %+v, want %+v", gotReq, wantReq)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if !gotAuthOK || gotAuthUser != "bob" || gotAuthPass != "hunter2" {
		t.Errorf("basic auth = (%q, %q, ok=%v), want (bob, hunter2, ok=true)", gotAuthUser, gotAuthPass, gotAuthOK)
	}
}

func TestXrayExportDetails_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	_, err := c.XrayExportDetails(context.Background(), "r", "i", "v")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error = %v, want mention of status 502", err)
	}
}

func TestXrayExportDetails_NoJSONInZip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, _ := zw.Create("readme.txt")
		_, _ = f.Write([]byte("no sbom here"))
		_ = zw.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = io.Copy(w, &buf)
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	_, err := c.XrayExportDetails(context.Background(), "r", "img", "1.0")
	if err == nil {
		t.Fatal("expected error when zip contains no JSON file")
	}
	if !strings.Contains(err.Error(), "no JSON file") {
		t.Errorf("error = %v, want mention of missing JSON", err)
	}
}

func TestXrayExportDetails_InvalidZip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a zip"))
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	_, err := c.XrayExportDetails(context.Background(), "r", "img", "1.0")
	if err == nil {
		t.Fatal("expected error for non-zip response body")
	}
	if !strings.Contains(err.Error(), "zip") {
		t.Errorf("error = %v, want mention of zip", err)
	}
}

func TestSearchVersions(t *testing.T) {
	const (
		repository = "libs-release"
		groupId    = "com.example.foo"
		artifactId = "bar"
	)

	var gotQuery url.Values
	var gotAuthUser, gotAuthPass string
	var gotAuthOK bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, gotAuthPass, gotAuthOK = r.BasicAuth()
		gotQuery = r.URL.Query()

		if r.URL.Path != "/artifactory/api/search/versions" {
			http.Error(w, fmt.Sprintf("unexpected path %q", r.URL.Path), http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "want GET", http.StatusMethodNotAllowed)
			return
		}

		_ = json.NewEncoder(w).Encode(VersionsResponse{
			Results: []struct {
				Version     string `json:"version"`
				Integration bool   `json:"integration"`
			}{
				{Version: "1.0.0", Integration: false},
				{Version: "1.1.0-SNAPSHOT", Integration: true},
				{Version: "1.1.0", Integration: false},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{
		JFrogURL: srv.URL,
		Auth:     Auth{Username: "alice", Password: "s3cret"},
	})

	versions, err := c.SearchVersions(context.Background(), repository, groupId, artifactId, false)
	if err != nil {
		t.Fatalf("SearchVersions: %v", err)
	}
	wantAll := []string{"1.0.0", "1.1.0-SNAPSHOT", "1.1.0"}
	if !slices.Equal(versions, wantAll) {
		t.Errorf("versions = %v, want %v", versions, wantAll)
	}
	if got := gotQuery.Get("g"); got != groupId {
		t.Errorf("query g = %q, want %q", got, groupId)
	}
	if got := gotQuery.Get("a"); got != artifactId {
		t.Errorf("query a = %q, want %q", got, artifactId)
	}
	if got := gotQuery.Get("repos"); got != repository {
		t.Errorf("query repos = %q, want %q", got, repository)
	}
	if !gotAuthOK || gotAuthUser != "alice" || gotAuthPass != "s3cret" {
		t.Errorf("basic auth = (%q, %q, ok=%v), want (alice, s3cret, ok=true)", gotAuthUser, gotAuthPass, gotAuthOK)
	}
}

func TestSearchVersions_ReleaseOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(VersionsResponse{
			Results: []struct {
				Version     string `json:"version"`
				Integration bool   `json:"integration"`
			}{
				{Version: "1.0.0", Integration: false},
				{Version: "1.1.0-SNAPSHOT", Integration: true},
				{Version: "1.1.0", Integration: false},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	versions, err := c.SearchVersions(context.Background(), "r", "g", "a", true)
	if err != nil {
		t.Fatalf("SearchVersions: %v", err)
	}
	want := []string{"1.0.0", "1.1.0"}
	if !slices.Equal(versions, want) {
		t.Errorf("versions = %v, want %v", versions, want)
	}
}

func TestSearchVersions_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	_, err := c.SearchVersions(context.Background(), "r", "g", "a", false)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want mention of status 500", err)
	}
}

func TestRetrieveArtifact(t *testing.T) {
	const repository = "libs-release"
	coords := MavenCoordinates{
		GroupID:    "com.example.foo",
		ArtifactID: "bar",
		Version:    "1.2.3",
		Extension:  "jar",
	}
	wantBody := []byte("artifact-binary-content")

	var gotPath string
	var gotAuthUser, gotAuthPass string
	var gotAuthOK bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, gotAuthPass, gotAuthOK = r.BasicAuth()
		gotPath = r.URL.Path

		if r.Method != http.MethodGet {
			http.Error(w, "want GET", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write(wantBody)
	}))
	defer srv.Close()

	c := NewClient(Config{
		JFrogURL: srv.URL,
		Auth:     Auth{Username: "alice", Password: "s3cret"},
	})

	var out bytes.Buffer
	if err := c.RetrieveArtifact(context.Background(), repository, coords, &out); err != nil {
		t.Fatalf("RetrieveArtifact: %v", err)
	}
	if !bytes.Equal(out.Bytes(), wantBody) {
		t.Errorf("body = %q, want %q", out.Bytes(), wantBody)
	}
	wantPath := "/artifactory/libs-release/com/example/foo/bar/1.2.3/bar-1.2.3.jar"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if !gotAuthOK || gotAuthUser != "alice" || gotAuthPass != "s3cret" {
		t.Errorf("basic auth = (%q, %q, ok=%v), want (alice, s3cret, ok=true)", gotAuthUser, gotAuthPass, gotAuthOK)
	}
}

func TestRetrieveArtifact_WithClassifier(t *testing.T) {
	coords := MavenCoordinates{
		GroupID:    "com.example",
		ArtifactID: "bar",
		Version:    "1.0",
		Classifier: "sources",
		Extension:  "jar",
	}

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	if err := c.RetrieveArtifact(context.Background(), "libs-release", coords, io.Discard); err != nil {
		t.Fatalf("RetrieveArtifact: %v", err)
	}
	wantPath := "/artifactory/libs-release/com/example/bar/1.0/bar-1.0-sources.jar"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
}

func TestRetrieveArtifact_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(Config{JFrogURL: srv.URL})
	coords := MavenCoordinates{GroupID: "g", ArtifactID: "a", Version: "1", Extension: "jar"}
	err := c.RetrieveArtifact(context.Background(), "r", coords, io.Discard)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want mention of status 404", err)
	}
}
