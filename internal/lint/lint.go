package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
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

	// If true, the linter will check the status of all entities for
	// "swcat-lint/finding-*" observations and report those as findings.
	CheckStatus bool `yaml:"checkStatus"`

	// If true, the linter will, for each Component, compare its observed
	// dependencies (status observations under "swcat-deps/*") against its
	// actual neighbors (declared dependencies and components reachable through a
	// shared API) and report observed dependencies that are not reflected in the
	// catalog as findings. See checkDependencyCandidates. This check needs a
	// resolver (see Linter.LintWithResolver) and is skipped without one.
	CheckDependencyCandidates bool `yaml:"checkDependencyCandidates"`

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

type DisplayLabel struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
}

// Regexp is a regexp.Regexp that unmarshals from a YAML string as a full match:
// the pattern is anchored with ^(?:...)$ so it must match the entire input.
type Regexp regexp.Regexp

// UnmarshalYAML compiles the YAML string value into a full-match regular expression.
func (r *Regexp) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	re, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return fmt.Errorf("failed to compile regexp %q: %w", s, err)
	}
	*r = Regexp(*re)
	return nil
}

// MatchString reports whether s is fully matched by the regexp.
func (r *Regexp) MatchString(s string) bool {
	return (*regexp.Regexp)(r).MatchString(s)
}

// matchesAny reports whether s is matched by any of the given regexps.
func matchesAny(res []*Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// PrometheusConfig holds lint-level configuration for Prometheus workload scanning.
type PrometheusConfig struct {
	// Enabled controls whether the workload scan is active. Defaults to false.
	Enabled bool `yaml:"enabled"`
	// The PromQL instant query to run to find workloads.
	WorkloadsQuery string `yaml:"workloadsQuery"`
	// The name of the label that identifies the workload name (e.g. "app", "label_app").
	WorkloadNameLabel string `yaml:"workloadNameLabel"`
	// Labels from the query result to display in the UI.
	DisplayLabels []DisplayLabel `yaml:"displayLabels"`
	// Whether to show the numeric value of the metric in the UI.
	ShowMetrics bool `yaml:"showMetrics"`
	// ExcludedWorkloads lists regular expressions matching workload names to
	// ignore. Each pattern must match the whole workload name, not a substring.
	ExcludedWorkloads []*Regexp `yaml:"excludedWorkloads,omitempty"`
	// WorkloadNameAnnotation is the catalog annotation used to match workload names.
	// Defaults to catalog.AnnotKubeName if empty.
	WorkloadNameAnnotation string `yaml:"workloadNameAnnotation,omitempty"`
}

// KubeConfig holds lint-level configuration for Kubernetes workload scanning.
type KubeConfig struct {
	// Enabled controls whether the workload scan is active. Defaults to false.
	Enabled bool `yaml:"enabled"`
	// ExcludedWorkloads lists regular expressions matching workload names to
	// ignore across all namespaces. Each pattern must match the whole workload
	// name, not a substring.
	ExcludedWorkloads []*Regexp `yaml:"excludedWorkloads,omitempty"`
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

// Linter provides efficient evaluation of rules against multiple entities.
type Linter struct {
	env            *cel.Env
	celRules       []compiledRule
	config         *Config
	reportedGroups []string // parsed/validated form of config.ReportedGroups
	bbCache        bbFilesCache
}

// NewLinter creates a new Linter from the given configuration.
// CEL expressions are compiled once and cached for subsequent evaluations.
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

	if config.Bitbucket.Enabled {
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
	}

	if config.Prometheus.Enabled {
		if strings.TrimSpace(config.Prometheus.WorkloadsQuery) == "" || config.Prometheus.WorkloadNameLabel == "" {
			return nil, fmt.Errorf("prometheus config is missing required fields [url, workloadsQuery, workloadNameLabel]")
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

// IsExcludedKubeWorkload reports whether the given Kubernetes workload name
// matches any of the configured Kube.ExcludedWorkloads regexes.
func (l *Linter) IsExcludedKubeWorkload(name string) bool {
	return matchesAny(l.config.Kube.ExcludedWorkloads, name)
}

// IsExcludedPrometheusWorkload reports whether the given Prometheus workload name
// matches any of the configured Prometheus.ExcludedWorkloads regexes.
func (l *Linter) IsExcludedPrometheusWorkload(name string) bool {
	return matchesAny(l.config.Prometheus.ExcludedWorkloads, name)
}

func (l *Linter) NumRules() int {
	return len(l.celRules)
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

// Resolver looks up catalog entities by reference. Graph-based lint checks
// (currently the dependency-candidate check) need it to traverse the entity
// graph, e.g. to find the consumers/providers of an API. *repo.Repository
// satisfies this interface.
type Resolver interface {
	Entity(ref *catalog.Ref) catalog.Entity
}

// Lint evaluates all applicable per-entity rules against the given entity.
// Graph-based checks that require resolving references (the dependency-candidate
// check) are skipped; use LintWithResolver to enable them.
func (l *Linter) Lint(e catalog.Entity) []Finding {
	return l.LintWithResolver(e, nil)
}

// LintWithResolver evaluates all applicable rules against the given entity. When
// resolver is non-nil, graph-based checks (the dependency-candidate check) are
// also evaluated; resolver should be the repository the entity belongs to.
func (l *Linter) LintWithResolver(e catalog.Entity, resolver Resolver) []Finding {
	var findings []Finding

	if len(l.celRules) > 0 {
		pbe := catalog.ToPB(e)
		args := map[string]any{
			"kind":     pbe.Kind,
			"metadata": pbe.Metadata,
			"spec":     extractSpec(pbe),
		}

		for _, cr := range l.celRules {
			if !l.shouldEvaluate(cr.condition, args) {
				continue
			}
			findings = append(findings, l.evaluate(cr, args)...)
		}
	}

	if l.config.CheckStatus {
		findings = append(findings, CheckStatusLintFindings(e)...)
	}

	if l.config.CheckDependencyCandidates && resolver != nil {
		findings = append(findings, checkDependencyCandidates(e, resolver)...)
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

// CheckStatusLintFindings is a built-in lint rule that checks for the existence of
// annotations whose name starts with "swcat-lint/finding-" and returns their contents as lint findings.
// If the annotation's value is a JSON-encoded api.LintFinding, its message is used;
// otherwise, the raw annotation value is returned as the finding message.
func CheckStatusLintFindings(e catalog.Entity) []Finding {
	status := e.GetStatus()
	if status == nil {
		return nil
	}

	var findings []Finding
	for name, obs := range status.Observations {
		if !strings.HasPrefix(name, "swcat-lint/finding-") {
			continue
		}
		f := Finding{
			RuleName: name,
			Severity: SeverityInfo,
		}
		// Try to parse observation value as a LintFinding.
		var finding api.LintFinding
		dec := json.NewDecoder(bytes.NewReader(obs.Value))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&finding); err == nil {
			f.Message = finding.Message
			if finding.Severity != "" {
				f.Severity = Severity(finding.Severity)
			}
		} else {
			// Not a structured finding message: use the whole text as message.
			f.Message = string(obs.Value)
		}
		findings = append(findings, f)
	}
	return findings
}
