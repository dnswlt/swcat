package repo

import (
	"testing"
	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
)

func mustNewEntityFromString(t *testing.T, s string) catalog.Entity {
	t.Helper()
	apiEnt, err := api.NewEntityFromString(s)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}
	ce, err := catalog.NewEntityFromAPI(apiEnt)
	if err != nil {
		t.Fatalf("Failed to convert API entity to catalog entity: %v", err)
	}
	return ce
}

// TestInsertOrUpdateEntity_PreservesExtensions verifies that sidecar extensions (stored in r.extensions)
// are re-applied to all entities when the repository is rebuilt during InsertOrUpdateEntity.
func TestInsertOrUpdateEntity_PreservesExtensions(t *testing.T) {
	const yaml1 = `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: c1
spec:
  type: service
  lifecycle: production
  owner: group:default/g1
  system: system:default/s1
`
	c1 := mustNewEntityFromString(t, yaml1)

	repo := NewRepository()

	// Helper to add entity from YAML
	addYAML := func(y string) catalog.Entity {
		ce := mustNewEntityFromString(t, y)
		if err := repo.AddEntity(ce); err != nil {
			t.Fatalf("Failed to add to repo: %v", err)
		}
		return ce
	}

	addYAML(`
apiVersion: swcat/v1alpha1
kind: Group
metadata:
  name: g1
spec:
  type: team
  profile:
    displayName: G1
`)
	addYAML(`
apiVersion: swcat/v1alpha1
kind: Domain
metadata:
  name: d1
spec:
  owner: group:default/g1
`)
	addYAML(`
apiVersion: swcat/v1alpha1
kind: System
metadata:
  name: s1
spec:
  owner: group:default/g1
  domain: domain:default/d1
`)

	// Add extension to repo
	repo.extensions.Entities[c1.GetRef().String()] = &api.MetadataExtensions{
		Annotations: map[string]any{"ext-annot": "ext-value"},
	}
	if err := repo.AddEntity(c1); err != nil {
		t.Fatalf("Failed to add c1 to repo: %v", err)
	}

	// Now update by adding c2
	const yaml2 = `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: c2
spec:
  type: service
  lifecycle: production
  owner: group:default/g1
  system: system:default/s1
`
	c2 := mustNewEntityFromString(t, yaml2)

	repo2, err := repo.InsertOrUpdateEntity(c2)
	if err != nil {
		t.Fatalf("InsertOrUpdateEntity failed: %v", err)
	}

	// Check if c1 still has the extension in repo2
	c1After := repo2.Component(c1.GetRef())
	if c1After == nil {
		t.Fatal("c1 not found in repo2")
	}
	if val, ok := c1After.GetMetadata().Annotations["ext-annot"]; !ok || val != `"ext-value"` {
		t.Errorf("c1 lost extension annot: got %q, want %q (ok=%t)", val, `"ext-value"`, ok)
	}
}
