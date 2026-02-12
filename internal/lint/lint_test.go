package lint

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"github.com/google/cel-go/cel"
)

func TestCELProtoEntityUX(t *testing.T) {
	env, err := cel.NewEnv(
		cel.Types(&catalog_pb.Entity{}),
		cel.Variable("kind", cel.StringType),
		cel.Variable("metadata", cel.ObjectType("swcat.catalog.v1.Metadata")),
		// We bind 'spec' as DynType so it can hold any of the *Spec messages.
		cel.Variable("spec", cel.DynType),
	)
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}

	// UX: User writes 'spec.type' directly.
	ast, iss := env.Compile(`kind == "component" && metadata.name == "my-service" && spec.type == "production"`)
	if iss.Err() != nil {
		t.Fatalf("Compile: %v", iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		t.Fatalf("Program: %v", err)
	}

	// Build a native catalog.Component
	comp := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "my-service",
		},
		Spec: &catalog.ComponentSpec{
			Type: "production",
		},
	}
	// Convert it to proto
	entity := catalog.ToPB(comp)

	// We pass the "flattened" fields to CEL.
	out, _, err := prg.Eval(map[string]any{
		"kind":     entity.Kind,
		"metadata": entity.Metadata,
		"spec":     extractSpec(entity),
	})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if out.Value() != true {
		t.Errorf("got %v, want true", out.Value())
	}
}

func TestCEL(t *testing.T) {

	env, err := cel.NewEnv(cel.Variable("name", cel.StringType))
	// Check err for environment setup errors.
	if err != nil {
		log.Fatalln(err)
	}
	ast, iss := env.Compile(`"Hello world! I'm " + name + "."`)
	// Check iss for compilation errors.
	if iss.Err() != nil {
		log.Fatalln(iss.Err())
	}
	prg, err := env.Program(ast)
	out, _, err := prg.Eval(map[string]interface{}{
		"name": "CEL",
	})
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}
	want := "Hello world! I'm CEL."
	if out.Value() != want {
		t.Errorf("got %v, want %q", out.Value(), want)
	}
}

// toMap converts a struct to a map[string]any via JSON round-trip,
// so that CEL expressions can use the JSON field names (e.g. spec.type).
func toMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	return m
}

func TestCELComponentViaJSON(t *testing.T) {
	env, err := cel.NewEnv(
		cel.Variable("spec", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}
	ast, iss := env.Compile(`spec.type == "banana"`)
	if iss.Err() != nil {
		t.Fatalf("Compile: %v", iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	tests := []struct {
		name     string
		specType string
		want     bool
	}{
		{"match", "banana", true},
		{"no match", "apple", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &catalog.Component{
				Metadata: &catalog.Metadata{Name: "test"},
				Spec:     &catalog.ComponentSpec{Type: tt.specType},
			}
			m := toMap(comp)
			out, _, err := prg.Eval(m)
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if out.Value() != tt.want {
				t.Errorf("got %v, want %v", out.Value(), tt.want)
			}
		})
	}
}

func TestLinter(t *testing.T) {
	config := &Config{
		CommonRules: []Rule{
			{
				Name:     "name-too-short",
				Severity: SeverityError,
				Check:    "metadata.name.size() > 3",
				Message:  "Entity name must be longer than 3 characters",
			},
		},
		KindRules: map[string][]Rule{
			"Component": {
				{
					Name:     "component-type-required",
					Severity: SeverityError,
					Check:    "spec.type != ''",
					Message:  "Component type is required",
				},
				{
					Name:      "production-needs-lifecycle",
					Severity:  SeverityWarn,
					Condition: "spec.type == 'service'",
					Check:     "spec.lifecycle == 'production'",
					Message:   "Services should ideally be in production",
				},
			},
		},
	}

	linter, err := NewLinter(config)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}

	tests := []struct {
		name     string
		entity   catalog.Entity
		wantRule []string
	}{
		{
			"valid component",
			&catalog.Component{
				Metadata: &catalog.Metadata{Name: "my-service"},
				Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production"},
			},
			nil,
		},
		{
			"name too short",
			&catalog.Component{
				Metadata: &catalog.Metadata{Name: "abc"},
				Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production"},
			},
			[]string{"name-too-short"},
		},
		{
			"missing type",
			&catalog.Component{
				Metadata: &catalog.Metadata{Name: "valid-name"},
				Spec:     &catalog.ComponentSpec{Type: "", Lifecycle: "production"},
			},
			[]string{"component-type-required"},
		},
		{
			"warn on lifecycle for service",
			&catalog.Component{
				Metadata: &catalog.Metadata{Name: "valid-name"},
				Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "experimental"},
			},
			[]string{"production-needs-lifecycle"},
		},
		{
			"no warn on lifecycle for library (condition check)",
			&catalog.Component{
				Metadata: &catalog.Metadata{Name: "valid-name"},
				Spec:     &catalog.ComponentSpec{Type: "library", Lifecycle: "experimental"},
			},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := catalog.ToPB(tt.entity)
			findings := linter.Lint(pb)

			if len(findings) != len(tt.wantRule) {
				t.Errorf("got %d findings, want %d", len(findings), len(tt.wantRule))
				for _, f := range findings {
					t.Logf("  Finding: %s - %s", f.RuleName, f.Message)
				}
				return
			}

			for i, want := range tt.wantRule {
				if findings[i].RuleName != want {
					t.Errorf("finding %d: got rule %q, want %q", i, findings[i].RuleName, want)
				}
			}
		})
	}
}
