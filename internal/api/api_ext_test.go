package api

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCatalogExtensions_Merge(t *testing.T) {
	tests := []struct {
		name  string
		base  *CatalogExtensions
		other *CatalogExtensions
		want  *CatalogExtensions
	}{
		{
			name:  "nil other",
			base:  &CatalogExtensions{},
			other: nil,
			want:  &CatalogExtensions{},
		},
		{
			name: "merge new entity",
			base: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v1"}},
				},
			},
			other: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"b": {Annotations: map[string]any{"k2": "v2"}},
				},
			},
			want: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v1"}},
					"b": {Annotations: map[string]any{"k2": "v2"}},
				},
			},
		},
		{
			name: "overwrite existing entity",
			base: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v1"}},
				},
			},
			other: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v2", "k2": "v3"}},
				},
			},
			want: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v2", "k2": "v3"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.base.Merge(tt.other)
			if diff := cmp.Diff(tt.want, tt.base); diff != "" {
				t.Errorf("Merge() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
