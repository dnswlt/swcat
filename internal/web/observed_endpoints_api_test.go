package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"github.com/dnswlt/swcat/internal/database"
	"github.com/dnswlt/swcat/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
)

// Lifecycle of observed endpoints through the API: re-reporting replaces,
// an empty report clears, tools are isolated, and deletion is per tool across
// all entities. Every check reads both the in-memory catalog and the database.

const (
	testComponentRef = "component:test-component"
	testResourceRef  = "resource:test-resource"
)

func newObservationsTestServer(t *testing.T) *Server {
	t.Helper()
	// On-disk, not ":memory:": the pool's connections must share one database.
	db := database.NewSqlite(filepath.Join(t.TempDir(), "swcat-test.db"))
	t.Cleanup(func() { db.Close() })
	if err := database.RecreateTables(t.Context(), db, false); err != nil {
		t.Fatalf("RecreateTables: %v", err)
	}
	s, err := NewServer(ServerOptions{Addr: "127.0.0.1:0", BaseDir: "../..", DotPath: "dot"},
		store.NewDiskStore("../../testdata/test1"), WithDatabase(db))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.dotRunner = &fakeRunner{}
	return s
}

// postEndpoints reports one outbound event endpoint per address for source.
func postEndpoints(t *testing.T, s *Server, source, tool string, addresses ...string) {
	t.Helper()
	ref := catalog.MustParseRef(source)
	req := &catalog_pb.ObservedEndpoints{Source: catalog.RefToPB(ref), DetectedBy: tool}
	for _, a := range addresses {
		req.Endpoints = append(req.Endpoints, &catalog_pb.Endpoint{
			Address:      a,
			Direction:    catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_OUTBOUND,
			Counterparts: []*catalog_pb.Counterpart{{External: "com.example:other"}},
		})
	}
	body, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/catalog/observed-endpoints", strings.NewReader(string(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s/%s: status %d: %s", source, tool, rr.Code, rr.Body)
	}
}

func deleteEndpoints(t *testing.T, s *Server, tool string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/catalog/observed-endpoints/"+tool, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE %s: status %d: %s", tool, rr.Code, rr.Body)
	}
	return rr.Body.String()
}

// storedAddresses returns the addresses each tool reported for ref, keyed by
// tool, after checking that the in-memory catalog and the database agree.
func storedAddresses(t *testing.T, s *Server, ref string) map[string]string {
	t.Helper()
	fromObs := func(where string, obs map[string]catalog.Observation) map[string]string {
		got := map[string]string{}
		for key, o := range obs {
			tool, ok := strings.CutPrefix(key, catalog.ObservedEndpointsKeyPrefix+"/")
			if !ok {
				continue
			}
			oe, err := catalog.ParseObservedEndpoints(o.Value)
			if err != nil {
				t.Fatalf("%s: %s: %v", where, key, err)
			}
			var addrs []string
			for _, ep := range oe.Endpoints {
				addrs = append(addrs, ep.Address)
			}
			got[tool] = strings.Join(addrs, ",")
		}
		return got
	}

	data, err := s.loadStoreData("")
	if err != nil {
		t.Fatalf("loadStoreData: %v", err)
	}
	var memObs map[string]catalog.Observation
	if st := data.repo.Entity(catalog.MustParseRef(ref)).GetStatus(); st != nil {
		memObs = st.Observations
	}
	mem := fromObs("memory", memObs)

	dbObs, err := database.LoadObservations(t.Context(), s.db, ref)
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	db := fromObs("database", dbObs)

	if !mapsEqual(mem, db) {
		t.Fatalf("%s: memory %v and database %v disagree", ref, mem, db)
	}
	return mem
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || v != w {
			return false
		}
	}
	return true
}

func wantStored(t *testing.T, s *Server, ref string, want map[string]string) {
	t.Helper()
	if got := storedAddresses(t, s, ref); !mapsEqual(got, want) {
		t.Errorf("%s: stored %v, want %v", ref, got, want)
	}
}

func TestObservedEndpoints_ReportReplaces(t *testing.T) {
	s := newObservationsTestServer(t)
	postEndpoints(t, s, testComponentRef, "tool-a", "a/1", "a/2")
	postEndpoints(t, s, testComponentRef, "tool-a", "a/3")
	wantStored(t, s, testComponentRef, map[string]string{"tool-a": "a/3"})
}

func TestObservedEndpoints_EmptyReportClears(t *testing.T) {
	s := newObservationsTestServer(t)
	postEndpoints(t, s, testComponentRef, "tool-a", "a/1")
	postEndpoints(t, s, testComponentRef, "tool-a")
	// The observation stays, as a record that the tool saw no endpoints.
	wantStored(t, s, testComponentRef, map[string]string{"tool-a": ""})
}

func TestObservedEndpoints_ToolsAreIsolated(t *testing.T) {
	s := newObservationsTestServer(t)
	postEndpoints(t, s, testComponentRef, "tool-a", "a/1")
	postEndpoints(t, s, testComponentRef, "tool-b", "b/1")
	postEndpoints(t, s, testComponentRef, "tool-a", "a/2")
	wantStored(t, s, testComponentRef, map[string]string{"tool-a": "a/2", "tool-b": "b/1"})

	deleteEndpoints(t, s, "tool-a")
	wantStored(t, s, testComponentRef, map[string]string{"tool-b": "b/1"})
}

func TestObservedEndpoints_DeleteFromAllEntities(t *testing.T) {
	s := newObservationsTestServer(t)
	postEndpoints(t, s, testComponentRef, "tool-a", "a/1")
	postEndpoints(t, s, testResourceRef, "tool-a", "a/2")
	postEndpoints(t, s, testResourceRef, "tool-b", "b/1")

	if msg := deleteEndpoints(t, s, "tool-a"); !strings.Contains(msg, "from 2 entities") {
		t.Errorf("DELETE response = %q, want it to report 2 entities", msg)
	}
	wantStored(t, s, testComponentRef, map[string]string{})
	wantStored(t, s, testResourceRef, map[string]string{"tool-b": "b/1"})

	// Deleting again is a no-op, not an error.
	if msg := deleteEndpoints(t, s, "tool-a"); !strings.Contains(msg, "from 0 entities") {
		t.Errorf("second DELETE response = %q, want it to report 0 entities", msg)
	}
}
