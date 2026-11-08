package catalog

import (
	"reflect"
	"testing"
)

// mapResolver resolves refs from a fixed map keyed by ref string.
type mapResolver map[string]Entity

func (m mapResolver) Entity(ref *Ref) Entity { return m[ref.String()] }

func TestDeclaredNeighbors(t *testing.T) {
	provided := &API{Metadata: &Metadata{Name: "provided"}, Spec: &APISpec{}}
	provided.AddConsumer(&LabelRef{Ref: MustParseRef("component:consumer")})
	provided.AddConsumer(&LabelRef{Ref: MustParseRef("component:c")}) // self: never a neighbor
	consumed := &API{Metadata: &Metadata{Name: "consumed"}, Spec: &APISpec{}}
	consumed.AddProvider(&LabelRef{Ref: MustParseRef("component:provider")})

	c := &Component{
		Metadata: &Metadata{Name: "c"},
		Spec: &ComponentSpec{
			DependsOn: []*LabelRef{
				{Ref: MustParseRef("component:dependency")},
				{Ref: MustParseRef("resource:db")}, // not a Component
			},
			ProvidesAPIs: []*LabelRef{{Ref: MustParseRef("api:provided")}},
			ConsumesAPIs: []*LabelRef{
				{Ref: MustParseRef("api:consumed")},
				{Ref: MustParseRef("api:unknown")}, // not resolvable: ignored
			},
		},
	}
	c.AddDependent(&LabelRef{Ref: MustParseRef("component:dependent")})

	got := DeclaredNeighbors(c, mapResolver{
		"api:provided": provided,
		"api:consumed": consumed,
	})
	want := map[string]bool{
		"component:dependency": true, // c depends on it
		"component:dependent":  true, // it depends on c
		"component:consumer":   true, // consumes an API c provides
		"component:provider":   true, // provides an API c consumes
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DeclaredNeighbors = %v, want %v", got, want)
	}
}
