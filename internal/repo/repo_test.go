package repo

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestRepository_AddAndGet(t *testing.T) {
	repo := NewRepository()

	tests := []struct {
		entity catalog.Entity
	}{
		{
			entity: &catalog.Component{Metadata: &catalog.Metadata{Name: "c1"}},
		},
		{
			entity: &catalog.System{Metadata: &catalog.Metadata{Name: "s1"}},
		},
		{
			entity: &catalog.Domain{Metadata: &catalog.Metadata{Name: "d1"}},
		},
		{
			entity: &catalog.API{Metadata: &catalog.Metadata{Name: "a1"}},
		},
		{
			entity: &catalog.Resource{Metadata: &catalog.Metadata{Name: "r1"}},
		},
		{
			entity: &catalog.Group{Metadata: &catalog.Metadata{Name: "g1"}},
		},
	}

	for _, tt := range tests {
		err := repo.AddEntity(tt.entity)
		if err != nil {
			t.Fatalf("AddEntity() with %s error = %v", tt.entity.GetKind(), err)
		}
	}

	if repo.Size() != len(tests) {
		t.Errorf("repo.Size() = %d, want %d", repo.Size(), len(tests))
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("get %s", tt.entity.GetKind()), func(t *testing.T) {
			var e catalog.Entity
			switch tt.entity.(type) {
			case *catalog.Component:
				e = repo.Component(tt.entity.GetRef())
			case *catalog.System:
				e = repo.System(tt.entity.GetRef())
			case *catalog.Domain:
				e = repo.Domain(tt.entity.GetRef())
			case *catalog.API:
				e = repo.API(tt.entity.GetRef())
			case *catalog.Resource:
				e = repo.Resource(tt.entity.GetRef())
			case *catalog.Group:
				e = repo.Group(tt.entity.GetRef())
			default:
				t.Fatalf("unknown typeName: %s", tt.entity.GetKind())
			}

			if e == nil {
				t.Fatalf("%s() returned nil", tt.entity.GetKind())
			}
			if !e.GetRef().Equal(tt.entity.GetRef()) {
				t.Errorf("e.GetRef().String() = %v, want %v", tt.entity.GetRef(), e.GetRef())
			}
		})
	}

	t.Run("add duplicate", func(t *testing.T) {
		err := repo.AddEntity(&catalog.Component{Metadata: &catalog.Metadata{Name: "c1"}})
		if err == nil {
			t.Error("AddEntity() error = nil, want error")
		}
	})
}

func TestRepository_Entity(t *testing.T) {
	repo := NewRepository()

	entities := []catalog.Entity{
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c1"}},
		&catalog.System{Metadata: &catalog.Metadata{Name: "s1"}},
		&catalog.Domain{Metadata: &catalog.Metadata{Name: "d1"}},
		&catalog.API{Metadata: &catalog.Metadata{Name: "a1"}},
		&catalog.Resource{Metadata: &catalog.Metadata{Name: "r1"}},
		&catalog.Group{Metadata: &catalog.Metadata{Name: "g1"}},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	for _, entity := range entities {
		t.Run(entity.GetMetadata().Name, func(t *testing.T) {
			e := repo.Entity(entity.GetRef())
			if e == nil {
				t.Fatal("Entity() returned nil")
			}
			if e.GetRef().String() != entity.GetRef().String() {
				t.Errorf("Entity().GetRef().String() = %s, want %s", e.GetRef().String(), entity.GetRef().String())
			}
		})
	}

	t.Run("non-existing ref", func(t *testing.T) {
		e := repo.Entity(&catalog.Ref{Kind: "component", Name: "s1"})
		if e != nil {
			t.Error("Entity() returned non-nil for non-existing ref")
		}
	})

	t.Run("invalid ref", func(t *testing.T) {
		e := repo.Entity(&catalog.Ref{Kind: "invalid", Name: "ref"})
		if e != nil {
			t.Error("Entity() returned non-nil for invalid ref")
		}
	})

	t.Run("ref without kind", func(t *testing.T) {
		e := repo.Entity(&catalog.Ref{Name: "c1"})
		if e != nil {
			t.Error("Entity() returned non-nil for ref without kind")
		}
	})
}

func TestRepository_Finders(t *testing.T) {
	repo := NewRepository()

	entities := []catalog.Entity{
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c2", Namespace: "ns1"}, Spec: &catalog.ComponentSpec{}}, // Add in different order
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c1", Namespace: "ns1"}, Spec: &catalog.ComponentSpec{}},
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c3", Namespace: "ns2"}, Spec: &catalog.ComponentSpec{}},
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c4", Namespace: "ns3"}, Spec: &catalog.ComponentSpec{
			Owner: &catalog.Ref{Name: "o4"}, Lifecycle: "production",
		}},
		&catalog.System{Metadata: &catalog.Metadata{Name: "s2"}, Spec: &catalog.SystemSpec{}},
		&catalog.System{Metadata: &catalog.Metadata{Name: "s1"}, Spec: &catalog.SystemSpec{}},
		&catalog.Domain{Metadata: &catalog.Metadata{Name: "d1"}, Spec: &catalog.DomainSpec{}},
		&catalog.API{Metadata: &catalog.Metadata{Name: "a1"}, Spec: &catalog.APISpec{}},
		&catalog.Resource{Metadata: &catalog.Metadata{Name: "r1"}, Spec: &catalog.ResourceSpec{}},
		&catalog.Group{Metadata: &catalog.Metadata{Name: "g2"}, Spec: &catalog.GroupSpec{}},
		&catalog.Group{Metadata: &catalog.Metadata{Name: "g1"}, Spec: &catalog.GroupSpec{}},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	type finderTest struct {
		query     string
		wantNames []string
	}

	testFinder := func(t *testing.T, finder func(string) []catalog.Entity, tests []finderTest) {
		for _, tt := range tests {
			t.Run(tt.query, func(t *testing.T) {
				results := finder(tt.query)
				if len(results) != len(tt.wantNames) {
					t.Errorf("len(results) = %d, want %d", len(results), len(tt.wantNames))
				}

				var gotNames []string
				for _, r := range results {
					gotNames = append(gotNames, r.GetRef().QName())
				}

				if !slices.Equal(gotNames, tt.wantNames) {
					t.Errorf("results = %v, want %v", gotNames, tt.wantNames)
				}
			})
		}
	}

	t.Run("FindComponents", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindComponents(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"ns1", []string{"ns1/c1", "ns1/c2"}},
			{"namespace:ns1 AND name:c1", []string{"ns1/c1"}},
			{"c1", []string{"ns1/c1"}},
			{"c3", []string{"ns2/c3"}},
			{"owner:o4 OR lifecycle:production", []string{"ns3/c4"}},
			{"notfound", nil},
			{"", []string{"ns1/c1", "ns1/c2", "ns2/c3", "ns3/c4"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindSystems", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindSystems(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"s", []string{"s1", "s2"}},
			{"s1", []string{"s1"}},
			{"", []string{"s1", "s2"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindDomains", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindDomains(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"d", []string{"d1"}},
			{"", []string{"d1"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindAPIs", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindAPIs(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"a", []string{"a1"}},
			{"", []string{"a1"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindResources", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindResources(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"r", []string{"r1"}},
			{"", []string{"r1"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindGroups", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindGroups(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"g", []string{"g1", "g2"}},
			{"g1", []string{"g1"}},
			{"", []string{"g1", "g2"}},
		}
		testFinder(t, finder, tests)
	})
}
