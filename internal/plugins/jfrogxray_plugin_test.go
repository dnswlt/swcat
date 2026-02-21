package plugins

import (
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"github.com/dnswlt/swcat/internal/repo"
)

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
	fullIdx := p.newCatalogIndexFromEntities(repository.AllEntities())

	t.Run("PerfectMatch", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.acme:beta:2.0.0",
			},
		}
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		if len(missing) != 0 || len(extra) != 0 {
			t.Errorf("expected 0 mismatches, got missing=%v, extra=%v", missing, extra)
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
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantMissing := []string{"org.example:gamma:1.0.0"}
		if !slices.Equal(missing, wantMissing) {
			t.Errorf("got missing=%v, want %v", missing, wantMissing)
		}
		if len(extra) != 0 {
			t.Errorf("got extra=%v, want empty", extra)
		}
	})

	t.Run("MissingInSBOM", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				// beta is missing
			},
		}
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantExtra := []string{"beta"}
		if !slices.Equal(extra, wantExtra) {
			t.Errorf("got extra=%v, want %v", extra, wantExtra)
		}
		if len(missing) != 0 {
			t.Errorf("got missing=%v, want empty", missing)
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
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantMissing := []string{"org.example:gamma:1.0.0"}
		wantExtra := []string{"beta"}
		if !slices.Equal(missing, wantMissing) {
			t.Errorf("got missing=%v, want %v", missing, wantMissing)
		}
		if !slices.Equal(extra, wantExtra) {
			t.Errorf("got extra=%v, want %v", extra, wantExtra)
		}
	})
}

func TestJFrogXrayPlugin_DetectDependencyMismatches_Ignore(t *testing.T) {
	repository := repo.NewRepository()

	e1 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "alpha"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	e2 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "beta"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	repository.AddEntity(e1)
	repository.AddEntity(e2)

	mainComp := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name:        "main",
			Annotations: make(map[string]string),
		},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
			DependsOn: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindComponent, Name: "alpha"}},
			},
		},
	}
	repository.AddEntity(mainComp)

	p := &JFrogXrayPlugin{
		spec: &jfrogXrayPluginSpec{
			LintIgnoreAnnotation: "my/ignore",
		},
	}
	fullIdx := p.newCatalogIndexFromEntities(repository.AllEntities())

	bom := &sbom.MiniBOM{
		Components: []string{
			"org.example:alpha:1.0.0",
			"org.example:beta:1.0.0", // beta is missing in mainComp deps
		},
	}

	t.Run("NoIgnore", func(t *testing.T) {
		missing, _ := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantMissing := []string{"org.example:beta:1.0.0"}
		if !slices.Equal(missing, wantMissing) {
			t.Errorf("got missing=%v, want %v", missing, wantMissing)
		}
	})

	t.Run("IgnoreByArtifactId", func(t *testing.T) {
		mainComp.Metadata.Annotations["my/ignore"] = `["beta"]`
		missing, _ := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		if len(missing) != 0 {
			t.Errorf("got missing=%v, want empty (ignored by artifactId)", missing)
		}
	})

	t.Run("IgnoreByGroupArtifactId", func(t *testing.T) {
		mainComp.Metadata.Annotations["my/ignore"] = `["org.example:beta"]`
		missing, _ := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		if len(missing) != 0 {
			t.Errorf("got missing=%v, want empty (ignored by groupId:artifactId)", missing)
		}
	})
}
