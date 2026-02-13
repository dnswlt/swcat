package lint

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"
)

type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
)

// Rule defines a single linting rule to be evaluated against catalog entities.
type Rule struct {
	// Name is a unique identifier for the rule.
	Name string `yaml:"name"`

	// Severity indicates the importance of the rule.
	// Defaults to "error" if unset.
	Severity Severity `yaml:"severity"`

	// Condition is an optional CEL expression.
	// If present, the rule is only evaluated if the condition returns true.
	Condition string `yaml:"condition,omitempty"`

	// Check is the CEL expression that validates the entity.
	// If it returns false, the rule is considered violated.
	Check string `yaml:"check"`

	// Message is the error message displayed when the check fails.
	Message string `yaml:"message"`
}

// Config represents the linting configuration, typically loaded from a lint.yaml file.
type Config struct {
	// CommonRules are applied to all entities, regardless of their Kind.
	CommonRules []Rule `yaml:"commonRules,omitempty"`

	// KindRules are applied only to entities of the specified Kind.
	// The map key is the entity Kind (e.g., "Component", "System").
	KindRules map[string][]Rule `yaml:"kindRules,omitempty"`

	// ReportedGroups is an optional list of group names (qualified names).
	// If set, the lint report will only show these groups as individual cards.
	// Findings for entities owned by other groups will be grouped under "Others".
	ReportedGroups []string `yaml:"reportedGroups,omitempty"`
}

// LoadConfig reads the linting configuration from the specified path.
func LoadConfig(path string) (*Config, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lint config %q: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(bs))
	dec.KnownFields(true)
	var config Config
	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid lint configuration YAML in %q: %w", path, err)
	}

	return &config, nil
}

// Finding represents a single rule violation.
type Finding struct {
	RuleName string
	Severity Severity
	Message  string
}

type compiledRule struct {
	rule      Rule
	condition cel.Program
	check     cel.Program
}

// Linter provides efficient evaluation of rules against multiple entities.
type Linter struct {
	env            *cel.Env
	commonRules    []compiledRule
	kindRules      map[string][]compiledRule
	reportedGroups []string
}

// NewLinter creates a new Linter from the given configuration.
// It compiles all CEL expressions once and caches them for subsequent evaluations.
func NewLinter(config *Config) (*Linter, error) {
	env, err := cel.NewEnv(
		cel.Types(&catalog_pb.Entity{}),
		cel.Variable("kind", cel.StringType),
		cel.Variable("metadata", cel.ObjectType("swcat.catalog.v1.Metadata")),
		cel.Variable("spec", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %v", err)
	}

	reportedGroups, err := parseReportedGroups(config.ReportedGroups)
	if err != nil {
		return nil, err
	}

	l := &Linter{
		env:            env,
		kindRules:      make(map[string][]compiledRule),
		reportedGroups: reportedGroups,
	}

	for _, r := range config.CommonRules {
		cr, err := l.compileRule(r)
		if err != nil {
			return nil, err
		}
		l.commonRules = append(l.commonRules, cr)
	}

	for kind, rules := range config.KindRules {
		lowerKind := strings.ToLower(kind)
		for _, r := range rules {
			cr, err := l.compileRule(r)
			if err != nil {
				return nil, err
			}
			l.kindRules[lowerKind] = append(l.kindRules[lowerKind], cr)
		}
	}

	return l, nil
}

func parseReportedGroups(groups []string) ([]string, error) {
	var result []string
	for _, rg := range groups {
		// Use catalog.KindGroup as the default kind for parsing reported groups.
		// api.ParseRef handles both "group:name" and just "name".
		r, err := catalog.ParseRef(rg)
		if err != nil {
			return nil, fmt.Errorf("invalid group reference in reportedGroups %q: %v", rg, err)
		}
		// If kind was omitted, assume KindGroup. If specified, ensure it is KindGroup.
		if r.Kind == "" {
			r.Kind = catalog.KindGroup
		} else if r.Kind != catalog.KindGroup {
			return nil, fmt.Errorf("invalid kind %q in reportedGroups %q: only Group entities are allowed", r.Kind, rg)
		}

		result = append(result, r.QName())
	}
	return result, nil
}

func (l *Linter) ReportedGroups() []string {
	return l.reportedGroups
}

func (l *Linter) compileRule(r Rule) (compiledRule, error) {
	var condition cel.Program
	if r.Condition != "" {
		ast, iss := l.env.Compile(r.Condition)
		if iss.Err() != nil {
			return compiledRule{}, fmt.Errorf("rule %q: condition compilation failed: %v", r.Name, iss.Err())
		}
		var err error
		condition, err = l.env.Program(ast)
		if err != nil {
			return compiledRule{}, fmt.Errorf("rule %q: condition program failed: %v", r.Name, err)
		}
	}

	ast, iss := l.env.Compile(r.Check)
	if iss.Err() != nil {
		return compiledRule{}, fmt.Errorf("rule %q: check compilation failed: %v", r.Name, iss.Err())
	}
	check, err := l.env.Program(ast)
	if err != nil {
		return compiledRule{}, fmt.Errorf("rule %q: check program failed: %v", r.Name, err)
	}

	return compiledRule{
		rule:      r,
		condition: condition,
		check:     check,
	}, nil
}

// Lint evaluates all applicable rules against the given entity.
func (l *Linter) Lint(e *catalog_pb.Entity) []Finding {
	var findings []Finding

	args := map[string]any{
		"kind":     e.Kind,
		"metadata": e.Metadata,
		"spec":     extractSpec(e),
	}

	// Apply common rules.
	for _, cr := range l.commonRules {
		findings = append(findings, l.evaluate(cr, args)...)
	}

	// Apply kind-specific rules.
	if kr, ok := l.kindRules[strings.ToLower(e.Kind)]; ok {
		for _, r := range kr {
			findings = append(findings, l.evaluate(r, args)...)
		}
	}

	return findings
}

func (l *Linter) evaluate(cr compiledRule, args map[string]any) []Finding {
	if cr.condition != nil {
		out, _, err := cr.condition.Eval(args)
		if err != nil {
			// Skip rule if condition evaluation fails.
			return nil
		}
		if out.Value() != true {
			return nil
		}
	}

	out, _, err := cr.check.Eval(args)
	if err != nil {
		// Report violation if check evaluation fails (e.g., due to missing field access).
		return []Finding{
			{
				RuleName: cr.rule.Name,
				Severity: SeverityError,
				Message:  fmt.Sprintf("Rule evaluation error: %v", err),
			},
		}
	}

	if out.Value() != true {
		severity := cr.rule.Severity
		if severity == "" {
			severity = SeverityError
		}
		return []Finding{
			{
				RuleName: cr.rule.Name,
				Severity: severity,
				Message:  cr.rule.Message,
			},
		}
	}

	return nil
}

func extractSpec(e *catalog_pb.Entity) any {
	switch s := e.Spec.(type) {
	case *catalog_pb.Entity_DomainSpec:
		return s.DomainSpec
	case *catalog_pb.Entity_SystemSpec:
		return s.SystemSpec
	case *catalog_pb.Entity_ComponentSpec:
		return s.ComponentSpec
	case *catalog_pb.Entity_ResourceSpec:
		return s.ResourceSpec
	case *catalog_pb.Entity_ApiSpec:
		return s.ApiSpec
	case *catalog_pb.Entity_GroupSpec:
		return s.GroupSpec
	}
	return nil
}
