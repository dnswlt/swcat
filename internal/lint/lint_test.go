package lint

import (
	"cmp"
	"encoding/json"
	"log"
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"github.com/google/cel-go/cel"
	gocmp "github.com/google/go-cmp/cmp"
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

func TestLinter(t *testing.T) {
	config := &Config{
		CELRules: []Rule{
			{
				Name:     "name-too-short",
				Severity: SeverityError,
				Check:    "metadata.name.size() > 3",
				Message:  "Entity name must be longer than 3 characters",
			},
			{
				Name:      "component-type-required",
				Severity:  SeverityError,
				Condition: "kind == 'component'",
				Check:     "spec.type != ''",
				Message:   "Component type is required",
			},
			{
				Name:      "production-needs-lifecycle",
				Severity:  SeverityWarn,
				Condition: "kind == 'component' && spec.type == 'service'",
				Check:     "spec.lifecycle == 'production'",
				Message:   "Services should ideally be in production",
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
			findings := linter.Lint(tt.entity)

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

func componentWithStatus(obs map[string]catalog.Observation) *catalog.Component {
	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "test"},
		Spec:     &catalog.ComponentSpec{Type: "service"},
	}
	if obs != nil {
		catalog.MergeObservations(c, obs)
	}
	return c
}

func TestCheckStatusLintFindings(t *testing.T) {
	tests := []struct {
		name   string
		obs    map[string]catalog.Observation
		useObs bool // when true, set status with (possibly empty) obs map
		want   []Finding
	}{
		{
			name: "nil status returns nil",
			want: nil,
		},
		{
			name:   "no matching observation prefix",
			useObs: true,
			obs: map[string]catalog.Observation{
				"some-plugin/thing":   {Value: json.RawMessage(`"ignored"`)},
				"another/observation": {Value: json.RawMessage(`{"x":1}`)},
			},
			want: nil,
		},
		{
			name:   "structured finding with severity",
			useObs: true,
			obs: map[string]catalog.Observation{
				"swcat-lint/finding-x": {
					Value: json.RawMessage(`{"createTime":"2024-01-01T00:00:00Z","message":"bad thing","severity":"warn"}`),
				},
			},
			want: []Finding{
				{RuleName: "swcat-lint/finding-x", Severity: SeverityWarn, Message: "bad thing"},
			},
		},
		{
			name:   "structured finding without severity defaults to info",
			useObs: true,
			obs: map[string]catalog.Observation{
				"swcat-lint/finding-y": {
					Value: json.RawMessage(`{"createTime":"2024-01-01T00:00:00Z","message":"soft note","severity":""}`),
				},
			},
			want: []Finding{
				{RuleName: "swcat-lint/finding-y", Severity: SeverityInfo, Message: "soft note"},
			},
		},
		{
			name:   "non-JSON value uses raw bytes as message",
			useObs: true,
			obs: map[string]catalog.Observation{
				"swcat-lint/finding-raw": {Value: json.RawMessage(`not json at all`)},
			},
			want: []Finding{
				{RuleName: "swcat-lint/finding-raw", Severity: SeverityInfo, Message: "not json at all"},
			},
		},
		{
			name:   "JSON with unknown fields falls back to raw bytes",
			useObs: true,
			obs: map[string]catalog.Observation{
				"swcat-lint/finding-unk": {Value: json.RawMessage(`{"foo":"bar"}`)},
			},
			want: []Finding{
				{RuleName: "swcat-lint/finding-unk", Severity: SeverityInfo, Message: `{"foo":"bar"}`},
			},
		},
		{
			name:   "only matching observations produce findings",
			useObs: true,
			obs: map[string]catalog.Observation{
				"swcat-lint/finding-a": {
					Value: json.RawMessage(`{"createTime":"2024-01-01T00:00:00Z","message":"msg a","severity":"error"}`),
				},
				"swcat-lint/finding-b": {
					Value: json.RawMessage(`plain b`),
				},
				"other/annotation": {Value: json.RawMessage(`"ignored"`)},
			},
			want: []Finding{
				{RuleName: "swcat-lint/finding-a", Severity: SeverityError, Message: "msg a"},
				{RuleName: "swcat-lint/finding-b", Severity: SeverityInfo, Message: "plain b"},
			},
		},
	}

	byRuleName := func(a, b Finding) int { return cmp.Compare(a.RuleName, b.RuleName) }

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entity catalog.Entity
			if tt.useObs {
				entity = componentWithStatus(tt.obs)
			} else {
				entity = componentWithStatus(nil)
			}

			got := CheckStatusLintFindings(entity)
			slices.SortFunc(got, byRuleName)
			want := slices.Clone(tt.want)
			slices.SortFunc(want, byRuleName)

			if diff := gocmp.Diff(want, got); diff != "" {
				t.Errorf("CheckStatusLintFindings() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
