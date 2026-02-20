package plugins

import (
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"github.com/dnswlt/swcat/internal/repo"
)

func TestJFrogXrayPlugin_FilterByCatalogEntities(t *testing.T) {
	repository := repo.NewRepository()

	// Entity 1: Matched by name
	e1 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "alpha"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	// Entity 2: Matched by CoordsAnnotation
	e2 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "beta-component",
			Annotations: map[string]string{
				"my/coords": "org.acme:beta",
			},
		},
		Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	// Entity 3: Another one matched by name
	e3 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "gamma"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}

	repository.AddEntity(e1)
	repository.AddEntity(e2)
	repository.AddEntity(e3)

	t.Run("WithCoordsAnnotation", func(t *testing.T) {
		p := &JFrogXrayPlugin{
			spec: &jfrogXrayPluginSpec{
				CoordsAnnotation: "my/coords",
			},
		}

		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0", // Matches e1 by name
				"org.acme:beta:2.0.0",    // Matches e2 by CoordsAnnotation
				"gamma",                  // Matches e3 by name
				"org.example:delta:1.0.0", // Should NOT match
				"unknown:unknown:1.0",    // Should NOT match
				"wrong.group:beta:1.0",   // Should NOT match e2 because group differs
			},
		}

		bom.Components = p.filterByCatalogEntities(bom, repository)

		want := []string{"org.example:alpha:1.0.0", "org.acme:beta:2.0.0", "gamma"}
		if len(bom.Components) != len(want) {
			t.Fatalf("got %d components, want %d: %v", len(bom.Components), len(want), bom.Components)
		}
		for i, c := range want {
			if bom.Components[i] != c {
				t.Errorf("bom.Components[%d] = %q, want %q", i, bom.Components[i], c)
			}
		}
	})
}

func TestJFrogXrayPlugin_DetectDependencyMismatches(t *testing.T) {
	repository := repo.NewRepository()

	// Create some catalog entities
	e1 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "alpha"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	e2 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "beta",
			Annotations: map[string]string{
				"my/coords": "org.acme:beta",
			},
		},
		Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	e3 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "gamma"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}

	repository.AddEntity(e1)
	repository.AddEntity(e2)
	repository.AddEntity(e3)

	// Main component with some dependencies
	mainComp := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "main"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
			DependsOn: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindComponent, Name: "alpha"}},
				{Ref: &catalog.Ref{Kind: catalog.KindComponent, Name: "beta"}},
			},
		},
	}
	repository.AddEntity(mainComp)

	p := &JFrogXrayPlugin{
		spec: &jfrogXrayPluginSpec{
			CoordsAnnotation: "my/coords",
		},
	}

	t.Run("PerfectMatch", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.acme:beta:2.0.0",
			},
		}
		mismatches := p.detectDependencyMismatches(bom, mainComp, repository)
		if len(mismatches) != 0 {
			t.Errorf("expected 0 mismatches, got %v", mismatches)
		}
	})

	t.Run("MissingInCatalog", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.acme:beta:2.0.0",
				"org.example:gamma:1.0.0", // gamma is in catalog but not in main's deps
			},
		}
		mismatches := p.detectDependencyMismatches(bom, mainComp, repository)
		want := []string{"-org.example:gamma:1.0.0"}
		if !slices.Equal(mismatches, want) {
			t.Errorf("got %v, want %v", mismatches, want)
		}
	})

	t.Run("MissingInSBOM", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				// beta is missing
			},
		}
		mismatches := p.detectDependencyMismatches(bom, mainComp, repository)
		want := []string{"+beta"}
		if !slices.Equal(mismatches, want) {
			t.Errorf("got %v, want %v", mismatches, want)
		}
	})

	t.Run("BothWaysMismatch", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.example:gamma:1.0.0", // extra
				// beta missing
			},
		}
		mismatches := p.detectDependencyMismatches(bom, mainComp, repository)
		// Descending sort sorts '-' before '+'
		want := []string{"-org.example:gamma:1.0.0", "+beta"}
		if !slices.Equal(mismatches, want) {
			t.Errorf("got %v, want %v", mismatches, want)
		}
	})
}
