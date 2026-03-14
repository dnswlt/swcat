package api

import (
	"testing"
	"time"

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
			name: "update and add keys, preserve untouched keys",
			base: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v1", "preserved": "yes"}},
				},
			},
			other: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v2", "k2": "v3"}},
				},
			},
			want: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v2", "k2": "v3", "preserved": "yes"}},
				},
			},
		},
		{
			name: "nil value deletes key",
			base: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": "v1", "k2": "v2"}},
				},
			},
			other: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k1": nil}},
				},
			},
			want: &CatalogExtensions{
				Entities: map[string]*MetadataExtensions{
					"a": {Annotations: map[string]any{"k2": "v2"}},
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

func TestWrapUnwrapAnnotation(t *testing.T) {
	now := time.Date(2023, 10, 27, 10, 0, 0, 0, time.UTC)
	formattedTime := "2023-10-27 10:00:00"
	value := "some-data"

	tests := []struct {
		name     string
		meta     map[string]any
		wantMeta map[string]any
	}{
		{
			name: "no extra meta",
			meta: nil,
			wantMeta: map[string]any{
				"updateTime": formattedTime,
			},
		},
		{
			name: "one field",
			meta: map[string]any{"version": "v1"},
			wantMeta: map[string]any{
				"updateTime": formattedTime,
				"version":    "v1",
			},
		},
		{
			name: "mixed types",
			meta: map[string]any{"version": "v1", "retryCount": 3},
			wantMeta: map[string]any{
				"updateTime": formattedTime,
				"version":    "v1",
				"retryCount": 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapAnnotation(value, now, tt.meta)
			gotData, gotMeta, found := UnwrapAnnotation(wrapped)

			if !found {
				t.Errorf("UnwrapAnnotation() found = false, want true")
				return
			}
			if gotData != value {
				t.Errorf("UnwrapAnnotation() data = %v, want %v", gotData, value)
			}
			if diff := cmp.Diff(tt.wantMeta, gotMeta); diff != "" {
				t.Errorf("UnwrapAnnotation() meta mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
