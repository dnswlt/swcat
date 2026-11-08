package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/database"
)

// postObservation stores a plain observation under key for ref through the API.
func postObservation(t *testing.T, s *Server, ref, key string) {
	t.Helper()
	path := "/catalog/entities/" + url.PathEscape(ref) + "/observations/" + url.PathEscape(key)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path,
		strings.NewReader(`{"value": {"ok": true}, "producer": "test"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s: status %d: %s", path, rr.Code, rr.Body)
	}
}

// bulkDelete calls DELETE /catalog/observations with the given query.
func bulkDelete(t *testing.T, s *Server, query string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/catalog/observations?"+query, nil))
	return rr.Code, rr.Body.String()
}

// storedKeys returns the sorted observation keys of ref, after checking that
// the in-memory catalog and the database agree.
func storedKeys(t *testing.T, s *Server, ref string) []string {
	t.Helper()
	data, err := s.loadStoreData("")
	if err != nil {
		t.Fatalf("loadStoreData: %v", err)
	}
	var mem []string
	if st := data.repo.Entity(catalog.MustParseRef(ref)).GetStatus(); st != nil {
		for k := range st.Observations {
			mem = append(mem, k)
		}
	}
	dbObs, err := database.LoadObservations(t.Context(), s.db, ref)
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	var db []string
	for k := range dbObs {
		db = append(db, k)
	}
	slices.Sort(mem)
	slices.Sort(db)
	if !slices.Equal(mem, db) {
		t.Fatalf("%s: memory %v and database %v disagree", ref, mem, db)
	}
	return mem
}

func wantKeys(t *testing.T, s *Server, ref string, want ...string) {
	t.Helper()
	if got := storedKeys(t, s, ref); !slices.Equal(got, want) {
		t.Errorf("%s: keys %v, want %v", ref, got, want)
	}
}

type bulkDeleteResponse struct {
	DryRun  bool                  `json:"dryRun"`
	Deleted []deletedObservations `json:"deleted"`
}

func decodeBulkDelete(t *testing.T, body string) bulkDeleteResponse {
	t.Helper()
	var resp bulkDeleteResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode response %q: %v", body, err)
	}
	return resp
}

// seedObservations stores, on both test entities, two keys under the old
// "swcat-deps/" prefix, one key that only shares a string prefix with it, and
// an unrelated key.
func seedObservations(t *testing.T, s *Server) {
	t.Helper()
	for _, ref := range []string{testComponentRef, testResourceRef} {
		for _, key := range []string{"swcat-deps/tool-a", "swcat-deps/tool-b", "swcat-deps-other", "keep/me"} {
			postObservation(t, s, ref, key)
		}
	}
}

func TestBulkDeleteObservations_ByKey(t *testing.T) {
	s := newObservationsTestServer(t)
	seedObservations(t, s)

	code, body := bulkDelete(t, s, "key=swcat-deps/tool-a")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	resp := decodeBulkDelete(t, body)
	want := []deletedObservations{
		{Entity: testComponentRef, Keys: []string{"swcat-deps/tool-a"}},
		{Entity: testResourceRef, Keys: []string{"swcat-deps/tool-a"}},
	}
	if resp.DryRun || !slices.EqualFunc(resp.Deleted, want, deletedEqual) {
		t.Errorf("response %+v, want deleted %+v", resp, want)
	}
	for _, ref := range []string{testComponentRef, testResourceRef} {
		wantKeys(t, s, ref, "keep/me", "swcat-deps-other", "swcat-deps/tool-b")
	}
}

func TestBulkDeleteObservations_ByPrefix(t *testing.T) {
	s := newObservationsTestServer(t)
	seedObservations(t, s)

	code, body := bulkDelete(t, s, "prefix=swcat-deps/")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	resp := decodeBulkDelete(t, body)
	if len(resp.Deleted) != 2 || !slices.Equal(resp.Deleted[0].Keys, []string{"swcat-deps/tool-a", "swcat-deps/tool-b"}) {
		t.Errorf("response %+v", resp)
	}
	for _, ref := range []string{testComponentRef, testResourceRef} {
		wantKeys(t, s, ref, "keep/me", "swcat-deps-other")
	}

	// Nothing left to match: an empty list, not an error.
	_, body = bulkDelete(t, s, "prefix=swcat-deps/")
	if resp := decodeBulkDelete(t, body); resp.Deleted == nil || len(resp.Deleted) != 0 {
		t.Errorf("second delete: response %s, want an empty list", body)
	}
}

func TestBulkDeleteObservations_DryRun(t *testing.T) {
	s := newObservationsTestServer(t)
	seedObservations(t, s)

	_, body := bulkDelete(t, s, "prefix=swcat-deps/&dryRun=true")
	resp := decodeBulkDelete(t, body)
	if !resp.DryRun || len(resp.Deleted) != 2 {
		t.Errorf("response %+v, want a dry run listing 2 entities", resp)
	}
	for _, ref := range []string{testComponentRef, testResourceRef} {
		wantKeys(t, s, ref, "keep/me", "swcat-deps-other", "swcat-deps/tool-a", "swcat-deps/tool-b")
	}
}

func TestBulkDeleteObservations_InvalidQuery(t *testing.T) {
	s := newObservationsTestServer(t)
	seedObservations(t, s)

	for _, query := range []string{"", "key=a&prefix=b", "prefix=", "prefix=%20", "key=a&dryRun=maybe"} {
		if code, body := bulkDelete(t, s, query); code != http.StatusBadRequest {
			t.Errorf("query %q: status %d (%s), want 400", query, code, body)
		}
	}
	wantKeys(t, s, testComponentRef, "keep/me", "swcat-deps-other", "swcat-deps/tool-a", "swcat-deps/tool-b")
}

func deletedEqual(a, b deletedObservations) bool {
	return a.Entity == b.Entity && slices.Equal(a.Keys, b.Keys)
}

// Rows of entities that no loaded catalog contains (e.g. deleted ones) are
// deleted too, and reported.
func TestBulkDeleteObservations_EntityNotInCatalog(t *testing.T) {
	s := newObservationsTestServer(t)
	seedObservations(t, s)
	gone := &catalog.Component{Metadata: &catalog.Metadata{Name: "gone"}, Spec: &catalog.ComponentSpec{}}
	catalog.MergeObservations(gone, map[string]catalog.Observation{
		"swcat-deps/tool-a": {Value: json.RawMessage(`1`), Producer: "test"},
	})
	if err := database.StoreObservations(t.Context(), s.db, gone); err != nil {
		t.Fatalf("StoreObservations: %v", err)
	}

	_, body := bulkDelete(t, s, "key=swcat-deps/tool-a")
	var entities []string
	for _, d := range decodeBulkDelete(t, body).Deleted {
		entities = append(entities, d.Entity)
	}
	if want := []string{"component:gone", testComponentRef, testResourceRef}; !slices.Equal(entities, want) {
		t.Errorf("deleted from %v, want %v", entities, want)
	}
	if obs, err := database.LoadObservations(t.Context(), s.db, "component:gone"); err != nil || len(obs) != 0 {
		t.Errorf("component:gone still has observations %v (err %v)", obs, err)
	}
}

// A real delete clears the cached catalogs, so they reload without the deleted
// observations; a dry run keeps them.
func TestBulkDeleteObservations_ClearsCachedCatalogs(t *testing.T) {
	s := newObservationsTestServer(t)
	seedObservations(t, s)

	bulkDelete(t, s, "prefix=swcat-deps/&dryRun=true")
	if len(s.storeDataMap) == 0 {
		t.Error("dry run cleared the cached catalogs")
	}

	bulkDelete(t, s, "prefix=swcat-deps/")
	if len(s.storeDataMap) != 0 {
		t.Errorf("%d cached catalogs, want all cleared", len(s.storeDataMap))
	}
	// Reloaded from the database, without the deleted observations.
	wantKeys(t, s, testComponentRef, "keep/me", "swcat-deps-other")
}
