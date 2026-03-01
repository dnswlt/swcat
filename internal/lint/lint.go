package lint

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"slices"
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

// Rule defines a single CEL-based linting rule evaluated against catalog entities.
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

// CustomCheckFunc is a Go function that performs complex validation on an entity.
// It may return multiple findings, each with its own message and severity.
type CustomCheckFunc func(rule CustomRule, e *catalog_pb.Entity) []Finding

// CustomRule wires a registered CustomCheckFunc into the linter by name.
type CustomRule struct {
	// Name is a unique identifier for the rule, used in findings.
	Name string `yaml:"name"`

	// Severity indicates the importance of the rule.
	// If set, it overrides the severity of all findings returned by the function.
	Severity Severity `yaml:"severity,omitempty"`

	// Condition is an optional CEL expression.
	// If present, the rule is only evaluated if the condition returns true.
	Condition string `yaml:"condition,omitempty"`

	// Func is the key under which the CustomCheckFunc was registered.
	Func string `yaml:"func"`

	Params map[string]string `yaml:"params"`
}

// Config represents the linting configuration, typically loaded from a lint.yaml file.
type Config struct {
	// CELRules are evaluated via CEL expressions against every entity (use
	// condition to restrict to specific kinds).
	CELRules []Rule `yaml:"celRules,omitempty"`

	// CustomRules invoke registered Go functions for complex validation that
	// cannot be expressed cleanly in CEL.
	CustomRules []CustomRule `yaml:"customRules,omitempty"`

	// ReportedGroups is an optional list of group names (qualified names).
	// If set, the lint report will only show these groups as individual cards.
	// Findings for entities owned by other groups will be grouped under "Others".
	ReportedGroups []string `yaml:"reportedGroups,omitempty"`

	// Kube holds lint-level configuration for Kubernetes workload scanning.
	Kube KubeConfig `yaml:"kube,omitempty"`

	// Prometheus holds lint-level configuration for Prometheus workload scanning.
	Prometheus PrometheusConfig `yaml:"prometheus,omitempty"`

	// BitBucket holds lint-level configuration for BitBucket component and API scanning.
	Bitbucket BitbucketConfig `yaml:"bitbucket,omitempty"`
}

// PrometheusConfig holds lint-level configuration for Prometheus workload scanning.
type PrometheusConfig struct {
	// Enabled controls whether the workload scan is active. Defaults to false.
	Enabled bool `yaml:"enabled"`
	// ExcludedWorkloads lists workload names to ignore.
	ExcludedWorkloads []string `yaml:"excludedWorkloads,omitempty"`
	// WorkloadNameAnnotation is the catalog annotation used to match workload names.
	// Defaults to catalog.AnnotKubeName if empty.
	WorkloadNameAnnotation string `yaml:"workloadNameAnnotation,omitempty"`
}

// KubeConfig holds lint-level configuration for Kubernetes workload scanning.
type KubeConfig struct {
	// Enabled controls whether the workload scan is active. Defaults to false.
	Enabled bool `yaml:"enabled"`
	// ExcludedWorkloads lists workload names to ignore across all namespaces.
	ExcludedWorkloads []string `yaml:"excludedWorkloads,omitempty"`
}

// BitbucketPathQuery configures a single Bitbucket scan for a specific
// file by full path or pattern.
type BitbucketPathQuery struct {
	// The kind of entity expected to find with the query.
	// Must be one of ("Component", "API").
	Kind string `yaml:"kind"`
	// The full path relative to the repository root of the file to look for.
	// Example: "/src/main/resources/asyncapi.yaml".
	// The leading "/" is optional, the path is always interpreted as starting
	// from the repository root.
	Path string `yaml:"path"`
	// A regular expression to match against *all* files in a repository.
	// If set, files in all repos mentioned in Repositories are fully listed
	// and matched against the regex. This is a potentially very expensive
	// operation, so use it carefully and only on selected repos.
	// The main use case for this is to support the scanning of monorepos.
	// The main reason it exists is that Bitbucket does not expose its Search API
	// publicly.
	// If PathRegex is set, Path must be empty.
	PathRegex string `yaml:"pathRegex"`

	// The names of repositories to apply this query to.
	// This filter should be set if PathRegex is set.
	Repositories []string `yaml:"repositories"`
}

// BitbucketConfig holds lint-level configuration for Bitbucket component and API scanning.
type BitbucketConfig struct {
	// Enabled controls whether the Bitbucket scan is active. Defaults to false.
	Enabled bool `yaml:"enabled"`
	// The list of projects to scan for files.
	Projects []string `yaml:"projects"`
	// List of regex patterns of repository names to exclude from the scan.
	ExcludedRepos []string `yaml:"excludedRepos"`
	// Queries to run to find potential missing Component or API entities.
	Queries []BitbucketPathQuery `yaml:"queries,omitempty"`
}

// ReadConfig reads the linting configuration from the specified path.
func ReadConfig(path string) (*Config, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lint config %q: %w", path, err)
	}
	return ParseConfig(bs)
}

// ParseConfig parses the linting configuration from YAML bytes.
func ParseConfig(yamlData []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(yamlData))
	dec.KnownFields(true)
	var config Config
	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid lint configuration YAML: %w", err)
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

type compiledCustomRule struct {
	rule      CustomRule
	condition cel.Program
	fn        CustomCheckFunc
}

// Linter provides efficient evaluation of rules against multiple entities.
type Linter struct {
	env            *cel.Env
	celRules       []compiledRule
	customRules    []compiledCustomRule
	config         *Config
	reportedGroups []string // parsed/validated form of config.ReportedGroups
}

// NewLinter creates a new Linter from the given configuration.
// CEL expressions are compiled once and cached for subsequent evaluations.
// customChecks maps function names to their implementations; it may be nil
// if the config contains no customRules. NewLinter returns an error if any
// customRule references a name not present in customChecks.
func NewLinter(config *Config, customChecks map[string]CustomCheckFunc) (*Linter, error) {
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
		config:         config,
		reportedGroups: reportedGroups,
	}

	for _, r := range config.CELRules {
		cr, err := l.compileRule(r)
		if err != nil {
			return nil, err
		}
		l.celRules = append(l.celRules, cr)
	}

	for _, cr := range config.CustomRules {
		fn, ok := customChecks[cr.Func]
		if !ok {
			// Invalid function. Add existing ones to error message for simpler debugging.
			knownFunctions := make([]string, 0, len(customChecks))
			for k := range customChecks {
				knownFunctions = append(knownFunctions, k)
			}
			slices.Sort(knownFunctions)
			return nil, fmt.Errorf("custom rule %q references unknown function %q (available: %s)", cr.Name, cr.Func, strings.Join(knownFunctions, ", "))
		}
		ccr, err := l.compileCustomRule(cr, fn)
		if err != nil {
			return nil, err
		}
		l.customRules = append(l.customRules, ccr)
	}

	for _, r := range config.Bitbucket.ExcludedRepos {
		_, err := regexp.Compile(r)
		if err != nil {
			return nil, fmt.Errorf("invalid excludedRepos regex %q in bitbucket config: %v", r, err)
		}
	}
	for i, q := range config.Bitbucket.Queries {
		if l := strings.ToLower(q.Kind); l != "component" && l != "api" {
			return nil, fmt.Errorf("invalid kind in bitbucket queries: %q (must be component or api)", q.Kind)
		}
		if q.Path == "" && q.PathRegex == "" {
			return nil, fmt.Errorf("path and pathRegex are both empty in bitbucket.queries[%d]", i)
		} else if q.Path != "" && q.PathRegex != "" {
			return nil, fmt.Errorf("path and pathRegex are both set in bitbucket.queries[%d]", i)
		} else if q.PathRegex != "" {
			_, err := regexp.Compile(q.PathRegex)
			if err != nil {
				return nil, fmt.Errorf("invalid pathRegex in bitbucket.queries[%d]: %v", i, err)
			}
		}
	}
	return l, nil
}

func parseReportedGroups(groups []string) ([]string, error) {
	var result []string
	for _, rg := range groups {
		// Use catalog.KindGroup as the default kind for parsing reported groups.
		r, err := catalog.ParseRefAs(catalog.KindGroup, rg)
		if err != nil {
			return nil, fmt.Errorf("invalid group reference in reportedGroups %q: %v", rg, err)
		}
		result = append(result, r.QName())
	}
	return result, nil
}

func (l *Linter) ReportedGroups() []string {
	return l.reportedGroups
}

func (l *Linter) Kube() KubeConfig {
	return l.config.Kube
}

func (l *Linter) Prometheus() PrometheusConfig {
	return l.config.Prometheus
}

func (l *Linter) Bitbucket() BitbucketConfig {
	return l.config.Bitbucket
}

func (l *Linter) NumRules() int {
	return len(l.celRules) + len(l.customRules)
}

func (l *Linter) compileCondition(name, condition string) (cel.Program, error) {
	if condition == "" {
		return nil, nil
	}
	ast, iss := l.env.Compile(condition)
	if iss.Err() != nil {
		return nil, fmt.Errorf("rule %q: condition compilation failed: %v", name, iss.Err())
	}
	prog, err := l.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("rule %q: condition program failed: %v", name, err)
	}
	return prog, nil
}

func (l *Linter) compileRule(r Rule) (compiledRule, error) {
	condition, err := l.compileCondition(r.Name, r.Condition)
	if err != nil {
		return compiledRule{}, err
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

func (l *Linter) compileCustomRule(cr CustomRule, fn CustomCheckFunc) (compiledCustomRule, error) {
	condition, err := l.compileCondition(cr.Name, cr.Condition)
	if err != nil {
		return compiledCustomRule{}, err
	}
	return compiledCustomRule{
		rule:      cr,
		condition: condition,
		fn:        fn,
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

	for _, cr := range l.celRules {
		if !l.shouldEvaluate(cr.condition, args) {
			continue
		}
		findings = append(findings, l.evaluate(cr, args)...)
	}

	for _, cr := range l.customRules {
		if !l.shouldEvaluate(cr.condition, args) {
			continue
		}
		customFindings := cr.fn(cr.rule, e)
		if cr.rule.Severity != "" {
			// Overwrite severity with configured value
			for i := range customFindings {
				customFindings[i].Severity = cr.rule.Severity
			}
		}
		findings = append(findings, customFindings...)
	}

	return findings
}

func (l *Linter) shouldEvaluate(condition cel.Program, args map[string]any) bool {
	if condition == nil {
		return true
	}
	out, _, err := condition.Eval(args)
	if err != nil {
		// Skip rule if condition evaluation fails.
		return false
	}
	return out.Value() == true
}

func (l *Linter) evaluate(cr compiledRule, args map[string]any) []Finding {
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
