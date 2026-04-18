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
