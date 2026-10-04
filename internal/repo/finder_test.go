package repo

import (
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

// Whether a query is valid must not depend on the catalog: an empty catalog, or
// one where no entity reaches the broken part, reports the same errors.
func TestFinderQueryErrors(t *testing.T) {
	populated := NewRepository()
	// c consumes no APIs, so evaluation never enters consumesApis[...].
	c := &catalog.Component{Metadata: &catalog.Metadata{Name: "c"}, Spec: &catalog.ComponentSpec{}}
	if err := populated.AddEntity(c); err != nil {
		t.Fatal(err)
	}
	finder := NewFinder()
	tests := []struct {
		query string
		err   string // empty: no error
	}{
		{"", ""},
		{"name=c", ""},
		{"name=nomatch", ""},
		{"systems[components[name:'flights-search']", "expected ']'"},
		{"name~'[a-'", "invalid regular expression"},
		{"unknown=x", "unknown attribute"},
		{"consumesApis[unknown=x]", "unknown attribute"},
		{"name=nomatch AND unknown=x", "unknown attribute"},
	}
	for _, r := range []struct {
		name string
		repo *Repository
	}{{"empty", NewRepository()}, {"populated", populated}} {
		for _, tt := range tests {
			t.Run(r.name+"/"+tt.query, func(t *testing.T) {
				_, err := finder.FindEntities(r.repo, tt.query)
				if tt.err == "" {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("got error %v, want %q", err, tt.err)
				}
			})
		}
	}
}
