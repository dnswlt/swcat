package backstage

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/api"
)

func TestRepository_AddAndGet(t *testing.T) {
	repo := NewRepository()

	tests := []struct {
		entity   api.Entity
		getRef   string
		typeName string
	}{
		{
			entity:   &api.Component{Metadata: &api.Metadata{Name: "c1"}},
			getRef:   "c1",
			typeName: "Component",
		},
		{
			entity:   &api.System{Metadata: &api.Metadata{Name: "s1"}},
			getRef:   "s1",
			typeName: "System",
		},
		{
			entity:   &api.Domain{Metadata: &api.Metadata{Name: "d1"}},
			getRef:   "d1",
			typeName: "Domain",
		},
		{
			entity:   &api.API{Metadata: &api.Metadata{Name: "a1"}},
			getRef:   "a1",
			typeName: "API",
		},
		{
			entity:   &api.Resource{Metadata: &api.Metadata{Name: "r1"}},
			getRef:   "r1",
			typeName: "Resource",
		},
		{
			entity:   &api.Group{Metadata: &api.Metadata{Name: "g1"}},
			getRef:   "g1",
			typeName: "Group",
		},
	}

	for _, tt := range tests {
		err := repo.AddEntity(tt.entity)
		if err != nil {
			t.Fatalf("AddEntity() with %s error = %v", tt.typeName, err)
		}
	}

	if repo.Size() != len(tests) {
		t.Errorf("repo.Size() = %d, want %d", repo.Size(), len(tests))
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("get %s", tt.typeName), func(t *testing.T) {
			var e api.Entity
			switch tt.typeName {
			case "Component":
				e = repo.Component(tt.getRef)
			case "System":
				e = repo.System(tt.getRef)
			case "Domain":
				e = repo.Domain(tt.getRef)
			case "API":
				e = repo.API(tt.getRef)
			case "Resource":
				e = repo.Resource(tt.getRef)
			case "Group":
				e = repo.Group(tt.getRef)
			default:
				t.Fatalf("unknown typeName: %s", tt.typeName)
			}

			if e == nil {
				t.Fatalf("%s() returned nil", tt.typeName)
			}
			if e.GetMetadata().Name != tt.getRef {
				t.Errorf("%s().Metadata.Name = %s, want %s", tt.typeName, e.GetMetadata().Name, tt.getRef)
			}
		})
	}

	t.Run("add duplicate", func(t *testing.T) {
		err := repo.AddEntity(&api.Component{Metadata: &api.Metadata{Name: "c1"}})
		if err == nil {
			t.Error("AddEntity() error = nil, want error")
		}
	})
}

func TestRepository_Entity(t *testing.T) {
	repo := NewRepository()

	entities := []api.Entity{
		&api.Component{Metadata: &api.Metadata{Name: "c1"}},
		&api.System{Metadata: &api.Metadata{Name: "s1"}},
		&api.Domain{Metadata: &api.Metadata{Name: "d1"}},
		&api.API{Metadata: &api.Metadata{Name: "a1"}},
		&api.Resource{Metadata: &api.Metadata{Name: "r1"}},
		&api.Group{Metadata: &api.Metadata{Name: "g1"}},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	tests := []struct {
		ref  string
		name string
	}{
		{"component:c1", "c1"},
		{"system:s1", "s1"},
		{"domain:d1", "d1"},
		{"api:a1", "a1"},
		{"resource:r1", "r1"},
		{"group:g1", "g1"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			e := repo.Entity(tt.ref)
			if e == nil {
				t.Fatal("Entity() returned nil")
			}
			if e.GetMetadata().Name != tt.name {
				t.Errorf("Entity().GetMetadata().Name = %s, want %s", e.GetMetadata().Name, tt.name)
			}
		})
	}

	t.Run("invalid ref", func(t *testing.T) {
		e := repo.Entity("invalid:ref")
		if e != nil {
			t.Error("Entity() returned non-nil for invalid ref")
		}
	})

	t.Run("ref without kind", func(t *testing.T) {
		e := repo.Entity("c1")
		if e != nil {
			t.Error("Entity() returned non-nil for ref without kind")
		}
	})
}

func TestRepository_Finders(t *testing.T) {
	repo := NewRepository()

	entities := []api.Entity{
		&api.Component{Metadata: &api.Metadata{Name: "c2", Namespace: "ns1"}}, // Add in different order
		&api.Component{Metadata: &api.Metadata{Name: "c1", Namespace: "ns1"}},
		&api.Component{Metadata: &api.Metadata{Name: "c3", Namespace: "ns2"}},
		&api.Component{Metadata: &api.Metadata{Name: "c4", Namespace: "ns3"}, Spec: &api.ComponentSpec{
			Owner: "o4", Lifecycle: "production",
		}},
		&api.System{Metadata: &api.Metadata{Name: "s2"}},
		&api.System{Metadata: &api.Metadata{Name: "s1"}},
		&api.Domain{Metadata: &api.Metadata{Name: "d1"}},
		&api.API{Metadata: &api.Metadata{Name: "a1"}},
		&api.Resource{Metadata: &api.Metadata{Name: "r1"}},
		&api.Group{Metadata: &api.Metadata{Name: "g2"}},
		&api.Group{Metadata: &api.Metadata{Name: "g1"}},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	type finderTest struct {
		query     string
		wantNames []string
	}

	testFinder := func(t *testing.T, finder func(string) []api.Entity, tests []finderTest) {
		for _, tt := range tests {
			t.Run(tt.query, func(t *testing.T) {
				results := finder(tt.query)
				if len(results) != len(tt.wantNames) {
					t.Errorf("len(results) = %d, want %d", len(results), len(tt.wantNames))
				}

				var gotNames []string
				for _, r := range results {
					gotNames = append(gotNames, r.GetQName())
				}

				if !slices.Equal(gotNames, tt.wantNames) {
					t.Errorf("results = %v, want %v", gotNames, tt.wantNames)
				}
			})
		}
	}

	t.Run("FindComponents", func(t *testing.T) {
		finder := func(q string) []api.Entity {
			var entities []api.Entity
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
		finder := func(q string) []api.Entity {
			var entities []api.Entity
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
		finder := func(q string) []api.Entity {
			var entities []api.Entity
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
		finder := func(q string) []api.Entity {
			var entities []api.Entity
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
		finder := func(q string) []api.Entity {
			var entities []api.Entity
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
		finder := func(q string) []api.Entity {
			var entities []api.Entity
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
