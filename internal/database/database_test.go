package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/google/go-cmp/cmp"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := NewSqlite(":memory:")
	t.Cleanup(func() { db.Close() })
	if err := RecreateTables(context.Background(), db, false); err != nil {
		t.Fatalf("RecreateTables: %v", err)
	}
	return db
}

func newTestComponent(name string, obs map[string]catalog.Observation) *catalog.Component {
	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: name},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
		},
	}
	if obs != nil {
		catalog.MergeObservations(c, obs)
	}
	return c
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestStoreAndLoadObservations_Roundtrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	now := time.Now().UTC().Round(time.Nanosecond)
	obs := map[string]catalog.Observation{
		"swcat/foo": {
			Value:     mustJSON(t, "hello"),
			Producer:  "FooPlugin",
			UpdatedAt: now,
			Version:   "v1.0",
			Meta: map[string]string{
				"a": "1",
				"b": "2",
			},
		},
		"swcat/bar": {
			Value:     mustJSON(t, map[string]int{"n": 42}),
			Producer:  "BarPlugin",
			UpdatedAt: now.Add(-time.Minute),
			// No version, no meta
		},
	}
	c := newTestComponent("comp", obs)

	if err := StoreObservations(ctx, db, c); err != nil {
		t.Fatalf("StoreObservations: %v", err)
	}

	got, err := LoadObservations(ctx, db, c.GetRef().String())
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	if len(got) != len(obs) {
		t.Fatalf("got %d observations, want %d", len(got), len(obs) )
	}
	for key, want := range obs {
		g, ok := got[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if g.Producer != want.Producer {
			t.Errorf("%s: producer %q, want %q", key, g.Producer, want.Producer)
		}
		if !g.UpdatedAt.Equal(want.UpdatedAt) {
			t.Errorf("%s: updatedAt %v, want %v", key, g.UpdatedAt, want.UpdatedAt)
		}
		if string(g.Value) != string(want.Value) {
			t.Errorf("%s: value %q, want %q", key, g.Value, want.Value)
		}
		if diff := cmp.Diff(want.Meta, g.Meta); diff != "" {
			t.Errorf("%s: Meta mismatch (-want +got):\n%s", key, diff)
		}
	}
}

func TestStoreObservations_ReplaceSemantics(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	now := time.Now().UTC()
	c := newTestComponent("comp", map[string]catalog.Observation{
		"a": {Value: mustJSON(t, 1), Producer: "p", UpdatedAt: now},
		"b": {Value: mustJSON(t, 2), Producer: "p", UpdatedAt: now},
	})
	if err := StoreObservations(ctx, db, c); err != nil {
		t.Fatalf("StoreObservations: %v", err)
	}

	// Replace with a different set: "a" updated, "b" removed, "c" added.
	catalog.ReplaceObservations(c, map[string]catalog.Observation{
		"a": {Value: mustJSON(t, 10), Producer: "p2", UpdatedAt: now},
		"c": {Value: mustJSON(t, 3), Producer: "p", UpdatedAt: now},
	})
	if err := StoreObservations(ctx, db, c); err != nil {
		t.Fatalf("StoreObservations (replace): %v", err)
	}

	got, err := LoadObservations(ctx, db, c.GetRef().String())
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	if _, ok := got["b"]; ok {
		t.Errorf("key 'b' should have been removed")
	}
	if g, ok := got["a"]; !ok || g.Producer != "p2" || string(g.Value) != "10" {
		t.Errorf("key 'a' not updated; got %+v", g)
	}
	if _, ok := got["c"]; !ok {
		t.Errorf("key 'c' missing")
	}
	if len(got) != 2 {
		t.Errorf("got %d observations, want 2", len(got))
	}
}

func TestStoreObservations_EmptyClears(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	now := time.Now().UTC()
	c := newTestComponent("comp", map[string]catalog.Observation{
		"a": {Value: mustJSON(t, 1), Producer: "p", UpdatedAt: now},
	})
	if err := StoreObservations(ctx, db, c); err != nil {
		t.Fatalf("StoreObservations: %v", err)
	}

	// Clear Status entirely — should wipe the rows.
	catalog.ReplaceObservations(c, nil)
	if err := StoreObservations(ctx, db, c); err != nil {
		t.Fatalf("StoreObservations (empty): %v", err)
	}

	got, err := LoadObservations(ctx, db, c.GetRef().String())
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 observations, got %d: %v", len(got), got)
	}
}

func TestStoreObservations_EntityWithoutStatus(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	// Domain has no Status — StoreObservations should be a no-op, not error.
	d := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "dom"},
		Spec:     &catalog.DomainSpec{Owner: &catalog.Ref{Name: "owner"}},
	}
	if err := StoreObservations(ctx, db, d); err != nil {
		t.Fatalf("StoreObservations: %v", err)
	}

	got, err := LoadObservations(ctx, db, d.GetRef().String())
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 observations, got %d", len(got))
	}
}

func TestStoreObservations_Isolation(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	now := time.Now().UTC()
	c1 := newTestComponent("one", map[string]catalog.Observation{
		"k": {Value: mustJSON(t, "v1"), Producer: "p", UpdatedAt: now},
	})
	c2 := newTestComponent("two", map[string]catalog.Observation{
		"k": {Value: mustJSON(t, "v2"), Producer: "p", UpdatedAt: now},
	})
	if err := StoreObservations(ctx, db, c1); err != nil {
		t.Fatalf("StoreObservations c1: %v", err)
	}
	if err := StoreObservations(ctx, db, c2); err != nil {
		t.Fatalf("StoreObservations c2: %v", err)
	}

	// Clear c1's observations; c2's must stay.
	catalog.ReplaceObservations(c1, nil)
	if err := StoreObservations(ctx, db, c1); err != nil {
		t.Fatalf("StoreObservations clear c1: %v", err)
	}

	got1, _ := LoadObservations(ctx, db, c1.GetRef().String())
	got2, _ := LoadObservations(ctx, db, c2.GetRef().String())
	if len(got1) != 0 {
		t.Errorf("c1: expected 0, got %d", len(got1))
	}
	if len(got2) != 1 {
		t.Errorf("c2: expected 1, got %d", len(got2))
	}
}

func TestDeleteObservationsByKey(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	obs := func(keys ...string) map[string]catalog.Observation {
		m := map[string]catalog.Observation{}
		for _, k := range keys {
			m[k] = catalog.Observation{Value: mustJSON(t, "v"), Producer: "p", UpdatedAt: now}
		}
		return m
	}
	// "a%" checks that the prefix is not a LIKE pattern.
	seed := func(t *testing.T) *sql.DB {
		db := newTestDB(t)
		for _, c := range []*catalog.Component{
			newTestComponent("one", obs("deps/x", "deps/y", "deps-other", "a%b")),
			newTestComponent("two", obs("deps/x", "keep", "abc")),
		} {
			if err := StoreObservations(ctx, db, c); err != nil {
				t.Fatalf("StoreObservations: %v", err)
			}
		}
		return db
	}
	keysOf := func(t *testing.T, db *sql.DB, name string) []string {
		got, err := LoadObservations(ctx, db, "component:"+name)
		if err != nil {
			t.Fatalf("LoadObservations: %v", err)
		}
		var keys []string
		for k := range got {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		return keys
	}

	tests := []struct {
		name             string
		sel              KeySelector
		dryRun           bool
		wantDeleted      []EntityKeys
		wantOne, wantTwo []string
	}{
		{
			name: "key",
			sel:  KeySelector{Key: "deps/x"},
			wantDeleted: []EntityKeys{
				{EntityRef: "component:one", Keys: []string{"deps/x"}},
				{EntityRef: "component:two", Keys: []string{"deps/x"}},
			},
			wantOne: []string{"a%b", "deps-other", "deps/y"},
			wantTwo: []string{"abc", "keep"},
		},
		{
			name: "prefix",
			sel:  KeySelector{Prefix: "deps/"},
			wantDeleted: []EntityKeys{
				{EntityRef: "component:one", Keys: []string{"deps/x", "deps/y"}},
				{EntityRef: "component:two", Keys: []string{"deps/x"}},
			},
			wantOne: []string{"a%b", "deps-other"},
			wantTwo: []string{"abc", "keep"},
		},
		{
			name:        "prefix is not a LIKE pattern",
			sel:         KeySelector{Prefix: "a%"},
			wantDeleted: []EntityKeys{{EntityRef: "component:one", Keys: []string{"a%b"}}},
			wantOne:     []string{"deps-other", "deps/x", "deps/y"},
			wantTwo:     []string{"abc", "deps/x", "keep"},
		},
		{
			name:   "dry run",
			sel:    KeySelector{Prefix: "deps/"},
			dryRun: true,
			wantDeleted: []EntityKeys{
				{EntityRef: "component:one", Keys: []string{"deps/x", "deps/y"}},
				{EntityRef: "component:two", Keys: []string{"deps/x"}},
			},
			wantOne: []string{"a%b", "deps-other", "deps/x", "deps/y"},
			wantTwo: []string{"abc", "deps/x", "keep"},
		},
		{
			name:    "no match",
			sel:     KeySelector{Key: "nope"},
			wantOne: []string{"a%b", "deps-other", "deps/x", "deps/y"},
			wantTwo: []string{"abc", "deps/x", "keep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := seed(t)
			deleted, err := DeleteObservationsByKey(ctx, db, tt.sel, tt.dryRun)
			if err != nil {
				t.Fatalf("DeleteObservationsByKey: %v", err)
			}
			if diff := cmp.Diff(tt.wantDeleted, deleted); diff != "" {
				t.Errorf("deleted (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantOne, keysOf(t, db, "one")); diff != "" {
				t.Errorf("one: remaining keys (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantTwo, keysOf(t, db, "two")); diff != "" {
				t.Errorf("two: remaining keys (-want +got):\n%s", diff)
			}
		})
	}

	for _, sel := range []KeySelector{{}, {Key: "a", Prefix: "b"}} {
		if _, err := DeleteObservationsByKey(ctx, newTestDB(t), sel, false); err == nil {
			t.Errorf("selector %+v: want an error", sel)
		}
	}
}
