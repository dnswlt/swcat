package repo

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

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
		f := NewFinder()
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range f.FindComponents(repo, q) {
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
		f := NewFinder()
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range f.FindSystems(repo, q) {
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
		f := NewFinder()
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range f.FindDomains(repo, q) {
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
		f := NewFinder()
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range f.FindAPIs(repo, q) {
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
		f := NewFinder()
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range f.FindResources(repo, q) {
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
		f := NewFinder()
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range f.FindGroups(repo, q) {
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

func TestRepository_SpecFieldValues(t *testing.T) {
	repo := NewRepository()

	entities := []catalog.Entity{
		&catalog.Component{
			Metadata: &catalog.Metadata{Name: "c1"},
			Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production"},
		},
		&catalog.Component{
			Metadata: &catalog.Metadata{Name: "c2"},
			Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "experimental"},
		},
		&catalog.Component{
			Metadata: &catalog.Metadata{Name: "c3"},
			Spec:     &catalog.ComponentSpec{Type: "library", Lifecycle: "production"},
		},
		&catalog.API{
			Metadata: &catalog.Metadata{Name: "a1"},
			Spec:     &catalog.APISpec{Type: "openapi", Lifecycle: "deprecated"},
		},
		&catalog.Resource{
			Metadata: &catalog.Metadata{Name: "r1"},
			Spec:     &catalog.ResourceSpec{Type: "database"},
		},
		&catalog.System{
			Metadata: &catalog.Metadata{Name: "s1"},
			Spec:     &catalog.SystemSpec{Type: "legacy"},
		},
		&catalog.Domain{
			Metadata: &catalog.Metadata{Name: "d1"},
			Spec:     &catalog.DomainSpec{Type: "business"},
		},
		&catalog.Group{
			Metadata: &catalog.Metadata{Name: "g1"},
			Spec:     &catalog.GroupSpec{Type: "team"},
		},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	tests := []struct {
		kind      catalog.Kind
		field     string
		want      []string
		wantError bool
	}{
		{catalog.KindComponent, "type", []string{"library", "service"}, false},
		{catalog.KindComponent, "lifecycle", []string{"experimental", "production"}, false},
		{catalog.KindAPI, "type", []string{"openapi"}, false},
		{catalog.KindAPI, "lifecycle", []string{"deprecated"}, false},
		{catalog.KindResource, "type", []string{"database"}, false},
		{catalog.KindSystem, "type", []string{"legacy"}, false},
		{catalog.KindDomain, "type", []string{"business"}, false},
		{catalog.KindGroup, "type", []string{"team"}, false},
		{catalog.KindResource, "lifecycle", nil, true},
		{catalog.KindComponent, "invalid", nil, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.kind, tt.field), func(t *testing.T) {
			got, err := repo.SpecFieldValues(tt.kind, tt.field)
			if (err != nil) != tt.wantError {
				t.Fatalf("SpecFieldValues() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("SpecFieldValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRepository_SurroundingSystems(t *testing.T) {
	r := NewRepository()

	// External System 1
	sys1 := &catalog.System{Metadata: &catalog.Metadata{Name: "sys1"}, Spec: &catalog.SystemSpec{Owner: &catalog.Ref{Name: "o"}, Domain: &catalog.Ref{Name: "d"}}}
	comp1 := &catalog.Component{Metadata: &catalog.Metadata{Name: "comp1"}, Spec: &catalog.ComponentSpec{System: sys1.GetRef(), Owner: &catalog.Ref{Name: "o"}, Type: "service", Lifecycle: "production"}}
	api1 := &catalog.API{Metadata: &catalog.Metadata{Name: "api1"}, Spec: &catalog.APISpec{System: sys1.GetRef(), Owner: &catalog.Ref{Name: "o"}, Type: "openapi", Lifecycle: "production"}}

	// External System 2
	sys2 := &catalog.System{Metadata: &catalog.Metadata{Name: "sys2"}, Spec: &catalog.SystemSpec{Owner: &catalog.Ref{Name: "o"}, Domain: &catalog.Ref{Name: "d"}}}
	res2 := &catalog.Resource{Metadata: &catalog.Metadata{Name: "res2"}, Spec: &catalog.ResourceSpec{System: sys2.GetRef(), Owner: &catalog.Ref{Name: "o"}, Type: "database"}}

	// Target System
	targetSys := &catalog.System{Metadata: &catalog.Metadata{Name: "target"}, Spec: &catalog.SystemSpec{Owner: &catalog.Ref{Name: "o"}, Domain: &catalog.Ref{Name: "d"}}}
	targetComp := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "target-comp"},
		Spec: &catalog.ComponentSpec{
			System:    targetSys.GetRef(),
			Owner:     &catalog.Ref{Name: "o"},
			Type:      "service",
			Lifecycle: "production",
			ConsumesAPIs: []*catalog.LabelRef{
				{Ref: api1.GetRef()},
			},
			DependsOn: []*catalog.LabelRef{
				{Ref: res2.GetRef()},
			},
		},
	}
	targetAPI := &catalog.API{
		Metadata: &catalog.Metadata{Name: "target-api"},
		Spec: &catalog.APISpec{
			System:    targetSys.GetRef(),
			Owner:     &catalog.Ref{Name: "o"},
			Type:      "openapi",
			Lifecycle: "production",
		},
	}
	// Add a dependent from sys1 to target-api
	comp1.Spec.ConsumesAPIs = append(comp1.Spec.ConsumesAPIs, &catalog.LabelRef{Ref: targetAPI.GetRef()})

	// Add all to repo
	group := &catalog.Group{Metadata: &catalog.Metadata{Name: "o"}, Spec: &catalog.GroupSpec{Type: "team"}}
	domain := &catalog.Domain{Metadata: &catalog.Metadata{Name: "d"}, Spec: &catalog.DomainSpec{Owner: &catalog.Ref{Name: "o"}}}
	entities := []catalog.Entity{group, domain, sys1, comp1, api1, sys2, res2, targetSys, targetComp, targetAPI}
	for _, e := range entities {
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}

	// targetSys is related to:
	// - sys1 (via targetComp -> api1, and comp1 -> targetAPI)
	// - sys2 (via targetComp -> res2)
	surrounding := r.SurroundingSystems(targetSys)

	if len(surrounding) != 2 {
		t.Errorf("len(surrounding) = %d, want 2", len(surrounding))
	}

	var names []string
	for _, s := range surrounding {
		names = append(names, s.GetMetadata().Name)
	}
	slices.Sort(names)
	want := []string{"sys1", "sys2"}
	if !slices.Equal(names, want) {
		t.Errorf("surrounding systems = %v, want %v", names, want)
	}
}

func TestRepository_PopulateDomain(t *testing.T) {
	r := NewRepository()

	owner := &catalog.Ref{Kind: catalog.KindGroup, Name: "o"}
	domainRef := &catalog.Ref{Kind: catalog.KindDomain, Name: "d"}
	systemRef := &catalog.Ref{Kind: catalog.KindSystem, Name: "s"}

	g := &catalog.Group{Metadata: &catalog.Metadata{Name: "o"}, Spec: &catalog.GroupSpec{Type: "team"}}
	d := &catalog.Domain{Metadata: &catalog.Metadata{Name: "d"}, Spec: &catalog.DomainSpec{Owner: owner}}
	s := &catalog.System{Metadata: &catalog.Metadata{Name: "s"}, Spec: &catalog.SystemSpec{Owner: owner, Domain: domainRef}}

	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "c"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "prod", Owner: owner, System: systemRef},
	}
	res := &catalog.Resource{
		Metadata: &catalog.Metadata{Name: "r"},
		Spec:     &catalog.ResourceSpec{Type: "database", Owner: owner, System: systemRef},
	}
	a := &catalog.API{
		Metadata: &catalog.Metadata{Name: "a"},
		Spec:     &catalog.APISpec{Type: "openapi", Lifecycle: "stable", Owner: owner, System: systemRef},
	}

	entities := []catalog.Entity{g, d, s, c, res, a}
	for _, e := range entities {
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}

	if !c.GetDomain().Equal(domainRef) {
		t.Errorf("Component.GetDomain() = %v, want %v", c.GetDomain(), domainRef)
	}
	if !res.GetDomain().Equal(domainRef) {
		t.Errorf("Resource.GetDomain() = %v, want %v", res.GetDomain(), domainRef)
	}
	if !a.GetDomain().Equal(domainRef) {
		t.Errorf("API.GetDomain() = %v, want %v", a.GetDomain(), domainRef)
	}
}

// seedRepoWithComponent builds a minimal valid repository populated with
// Group g1, Domain d1, System s1 and Component c1. Entities are loaded
// from YAML so each one has the SourceInfo required by Reset().
func seedRepoWithComponent(t *testing.T) (r *Repository, c1 *catalog.Component) {
	t.Helper()
	r = NewRepository()
	add := func(y string) catalog.Entity {
		e := mustNewEntityFromString(t, y)
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
		return e
	}
	add(`
apiVersion: swcat/v1alpha1
kind: Group
metadata:
  name: g1
spec:
  type: team
  profile:
    displayName: Group One
`)
	add(`
apiVersion: swcat/v1alpha1
kind: Domain
metadata:
  name: d1
spec:
  owner: group:default/g1
`)
	add(`
apiVersion: swcat/v1alpha1
kind: System
metadata:
  name: s1
spec:
  owner: group:default/g1
  domain: domain:default/d1
`)
	e := add(`
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: c1
spec:
  type: service
  lifecycle: production
  owner: group:default/g1
  system: system:default/s1
`)
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	return r, e.(*catalog.Component)
}

func TestRepository_InsertOrUpdateEntity_Insert(t *testing.T) {
	r, c1 := seedRepoWithComponent(t)

	// Give c1 some status that must survive the repo rebuild.
	catalog.MergeObservations(c1, map[string]catalog.Observation{
		"health": {
			Value:     json.RawMessage(`"green"`),
			Producer:  "p",
			UpdatedAt: time.Now().UTC(),
		},
	})

	// Insert a brand-new component c2 that doesn't exist yet.
	c2 := mustNewEntityFromString(t, `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: c2
spec:
  type: service
  lifecycle: production
  owner: group:default/g1
  system: system:default/s1
`)

	r2, err := r.InsertOrUpdateEntity(c2)
	if err != nil {
		t.Fatalf("InsertOrUpdateEntity: %v", err)
	}

	// New entity is present in the new repo.
	if got := r2.Component(c2.GetRef()); got == nil {
		t.Fatalf("c2 missing in new repo after insert")
	}
	// Original repo is untouched.
	if got := r.Component(c2.GetRef()); got != nil {
		t.Errorf("original repo mutated: c2 unexpectedly present")
	}

	// c1 is still there and retains its status via the rebuild.
	got1 := r2.Component(c1.GetRef())
	if got1 == nil {
		t.Fatalf("c1 missing in new repo after insert")
	}
	status := got1.GetStatus()
	if status == nil || len(status.Observations) != 1 {
		t.Fatalf("c1 status not preserved: %+v", status)
	}
	if _, ok := status.Observations["health"]; !ok {
		t.Errorf("c1 observation %q missing after insert", "health")
	}

	// The carried-over status is a clone, not a shared reference: mutating
	// the new repo's copy must not leak into the original c1.
	catalog.MergeObservations(got1, map[string]catalog.Observation{
		"extra": {Value: json.RawMessage(`"x"`), Producer: "p"},
	})
	if _, ok := c1.GetStatus().Observations["extra"]; ok {
		t.Errorf("status not cloned: mutation on new repo leaked into original c1")
	}
}

func TestRepository_InsertOrUpdateEntity_Update(t *testing.T) {
	r, c1 := seedRepoWithComponent(t)

	// Attach status to the original c1.
	catalog.MergeObservations(c1, map[string]catalog.Observation{
		"health": {
			Value:     json.RawMessage(`"green"`),
			Producer:  "p",
			UpdatedAt: time.Now().UTC(),
		},
	})

	// Build an updated version with the same ref but a changed description and no status.
	c1New := mustNewEntityFromString(t, `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: c1
  description: updated
spec:
  type: service
  lifecycle: production
  owner: group:default/g1
  system: system:default/s1
`)
	if c1New.GetStatus() != nil {
		t.Fatalf("c1New should start without status")
	}

	r2, err := r.InsertOrUpdateEntity(c1New)
	if err != nil {
		t.Fatalf("InsertOrUpdateEntity: %v", err)
	}

	got := r2.Component(c1.GetRef())
	if got == nil {
		t.Fatalf("c1 missing in new repo after update")
	}
	if got.GetMetadata().Description != "updated" {
		t.Errorf("description = %q, want %q", got.GetMetadata().Description, "updated")
	}

	// Status was copied over from the old entity.
	status := got.GetStatus()
	if status == nil || len(status.Observations) != 1 {
		t.Fatalf("status not copied on update: %+v", status)
	}
	if _, ok := status.Observations["health"]; !ok {
		t.Errorf("observation %q missing after update", "health")
	}

	// The copy is independent: mutating the new entity's status must not
	// leak into the original c1 (which lives in the old repo r).
	catalog.MergeObservations(got, map[string]catalog.Observation{
		"extra": {Value: json.RawMessage(`"x"`), Producer: "p"},
	})
	if _, ok := c1.GetStatus().Observations["extra"]; ok {
		t.Errorf("status not cloned: mutation on new repo leaked into original c1")
	}

	// Original repo still holds the pre-update entity.
	if origGot := r.Component(c1.GetRef()); origGot != c1 {
		t.Errorf("original repo mutated: r.Component(c1) changed identity")
	}
}

func TestRepository_DeleteEntity_PreservesStatus(t *testing.T) {
	r, c1 := seedRepoWithComponent(t)

	// Attach status to c1 — this must survive the deletion of another entity.
	catalog.MergeObservations(c1, map[string]catalog.Observation{
		"health": {
			Value:     json.RawMessage(`"green"`),
			Producer:  "p",
			UpdatedAt: time.Now().UTC(),
		},
	})

	// Add a second component c2 that we can delete without violating constraints.
	c2 := mustNewEntityFromString(t, `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: c2
spec:
  type: service
  lifecycle: production
  owner: group:default/g1
  system: system:default/s1
`)
	r, err := r.InsertOrUpdateEntity(c2)
	if err != nil {
		t.Fatalf("InsertOrUpdateEntity(c2): %v", err)
	}

	r2, err := r.DeleteEntity(c2.GetRef())
	if err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}

	if got := r2.Component(c2.GetRef()); got != nil {
		t.Errorf("c2 still present after delete")
	}

	// c1's status must survive the rebuild.
	got1 := r2.Component(c1.GetRef())
	if got1 == nil {
		t.Fatalf("c1 missing after delete")
	}
	status := got1.GetStatus()
	if status == nil || len(status.Observations) != 1 {
		t.Fatalf("c1 status not preserved after delete: %+v", status)
	}
	if _, ok := status.Observations["health"]; !ok {
		t.Errorf("c1 observation %q missing after delete", "health")
	}
}
