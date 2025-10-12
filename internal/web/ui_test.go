package web

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/google/go-cmp/cmp"
)

func TestFormatLabels(t *testing.T) {
	type tc struct {
		name      string
		meta      *catalog.Metadata
		want      []string
		wantIsNil bool
	}

	cases := []tc{
		{
			name:      "nil meta",
			meta:      nil,
			want:      nil,
			wantIsNil: true,
		},
		{
			name: "no labels",
			meta: &catalog.Metadata{
				Labels: map[string]string{},
			},
			want:      nil,
			wantIsNil: true,
		},
		{
			name: "single unqualified label",
			meta: &catalog.Metadata{
				Labels: map[string]string{"env": "prod"},
			},
			want: []string{"env: prod"},
		},
		{
			name: "qualified unique tails -> show simple keys",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/role": "api",
					"acme.io/tier":     "gold",
				},
			},
			want: []string{
				"role: api",
				"tier: gold",
			},
		},
		{
			name: "ambiguous tails -> keep qualified keys",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/role": "api",
					"acme.io/role":     "db",
				},
			},
			want: []string{
				"acme.io/role: db",
				"example.com/role: api",
			},
		},
		{
			name: "mix of unique and ambiguous",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/role": "api",
					"acme.io/role":     "db",
					"region":           "eu-west",
				},
			},
			want: []string{
				"acme.io/role: db",
				"example.com/role: api",
				"region: eu-west",
			},
		},
		{
			name: "multiple slashes in key -> cut at first slash",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"a/b/c": "x",
					"d/e/f": "y",
				},
			},
			want: []string{
				"b/c: x",
				"e/f: y",
			},
		},
		{
			name: "empty values are allowed",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"team": "",
				},
			},
			want: []string{
				"team: ",
			},
		},
		{
			name: "stable order by displayed key then value",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/a": "2",
					"acme.io/a":     "1",
					"b":             "zz",
				},
			},
			want: []string{
				"acme.io/a: 1",
				"b: zz",
				"example.com/a: 2",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatLabels(c.meta)

			if c.wantIsNil {
				if got != nil {
					t.Fatalf("expected nil slice, got: %v", got)
				}
				return
			}

			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Fatalf("formatLabels() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
