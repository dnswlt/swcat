package api

import (
	"testing"
)

func TestMetadata_GetQName(t *testing.T) {
	tests := []struct {
		name     string
		metadata *Metadata
		want     string
	}{
		{
			name: "name only",
			metadata: &Metadata{
				Name: "my-component",
			},
			want: "my-component",
		},
		{
			name: "name and namespace",
			metadata: &Metadata{
				Name:      "my-component",
				Namespace: "my-namespace",
			},
			want: "my-namespace/my-component",
		},
		{
			name: "name and default namespace",
			metadata: &Metadata{
				Name:      "my-component",
				Namespace: "default",
			},
			want: "my-component",
		},
		{
			name: "name and empty namespace",
			metadata: &Metadata{
				Name:      "my-component",
				Namespace: "",
			},
			want: "my-component",
		},
		{
			name:     "nil metadata",
			metadata: nil,
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metadata.GetQName(); got != tt.want {
				t.Errorf("Metadata.GetQName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntity_GetQName(t *testing.T) {
	tests := []struct {
		name   string
		entity Entity
		want   string
	}{
		{
			name: "component",
			entity: &Component{
				Metadata: &Metadata{Name: "c", Namespace: "ns"},
			},
			want: "ns/c",
		},
		{
			name: "system",
			entity: &System{
				Metadata: &Metadata{Name: "s", Namespace: "ns"},
			},
			want: "ns/s",
		},
		{
			name: "domain",
			entity: &Domain{
				Metadata: &Metadata{Name: "d", Namespace: "ns"},
			},
			want: "ns/d",
		},
		{
			name: "api",
			entity: &API{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			want: "ns/a",
		},
		{
			name: "resource",
			entity: &Resource{
				Metadata: &Metadata{Name: "r", Namespace: "ns"},
			},
			want: "ns/r",
		},
		{
			name: "group",
			entity: &Group{
				Metadata: &Metadata{Name: "g", Namespace: "ns"},
			},
			want: "ns/g",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entity.GetQName(); got != tt.want {
				t.Errorf("Entity.GetQName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntity_GetRef(t *testing.T) {
	tests := []struct {
		name   string
		entity Entity
		want   string
	}{
		{
			name: "component",
			entity: &Component{
				Metadata: &Metadata{Name: "c", Namespace: "ns"},
			},
			want: "component:ns/c",
		},
		{
			name: "system",
			entity: &System{
				Metadata: &Metadata{Name: "s", Namespace: "ns"},
			},
			want: "system:ns/s",
		},
		{
			name: "domain",
			entity: &Domain{
				Metadata: &Metadata{Name: "d", Namespace: "ns"},
			},
			want: "domain:ns/d",
		},
		{
			name: "api",
			entity: &API{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			want: "api:ns/a",
		},
		{
			name: "resource",
			entity: &Resource{
				Metadata: &Metadata{Name: "r", Namespace: "ns"},
			},
			want: "resource:ns/r",
		},
		{
			name: "group",
			entity: &Group{
				Metadata: &Metadata{Name: "g", Namespace: "ns"},
			},
			want: "group:ns/g",
		},
		{
			name: "component without namespace",
			entity: &Component{
				Metadata: &Metadata{Name: "c"},
			},
			want: "component:c",
		},
		{
			name: "system without namespace",
			entity: &System{
				Metadata: &Metadata{Name: "s"},
			},
			want: "system:s",
		},
		{
			name: "domain without namespace",
			entity: &Domain{
				Metadata: &Metadata{Name: "d"},
			},
			want: "domain:d",
		},
		{
			name: "api without namespace",
			entity: &API{
				Metadata: &Metadata{Name: "a"},
			},
			want: "api:a",
		},
		{
			name: "resource without namespace",
			entity: &Resource{
				Metadata: &Metadata{Name: "r"},
			},
			want: "resource:r",
		},
		{
			name: "group without namespace",
			entity: &Group{
				Metadata: &Metadata{Name: "g"},
			},
			want: "group:g",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entity.GetRef(); got != tt.want {
				t.Errorf("Entity.GetRef() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareEntityByName(t *testing.T) {
	tests := []struct {
		name string
		a    Entity
		b    Entity
		want int
	}{
		{
			name: "equal",
			a: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			b: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			want: 0,
		},
		{
			name: "different names",
			a: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			b: &Component{
				Metadata: &Metadata{Name: "b", Namespace: "ns"},
			},
			want: -1,
		},
		{
			name: "different namespaces",
			a: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns1"},
			},
			b: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns2"},
			},
			want: -1,
		},
		{
			name: "one with namespace, one without",
			a: &Component{
				Metadata: &Metadata{Name: "a"},
			},
			b: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			want: -1,
		},
		{
			name: "swapped one with namespace, one without",
			a: &Component{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
			},
			b: &Component{
				Metadata: &Metadata{Name: "a"},
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareEntityByName(tt.a, tt.b); got != tt.want {
				t.Errorf("CompareEntityByName() = %v, want %v", got, tt.want)
			}
		})
	}
}
