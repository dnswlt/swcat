package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/lint"
)

type lintGroup struct {
	Name     string
	Entities []lintEntityFindings
}

type lintEntityFindings struct {
	Entity   catalog.Entity
	Findings []lint.Finding
}

type ownerLintGroups struct {
	Owner      string
	Systems    []lintGroup
	ErrorCount int
	WarnCount  int
	InfoCount  int
}

func (s *Server) serveLintFindings(w http.ResponseWriter, r *http.Request) {
	if s.linter == nil {
		http.Error(w, "Linter not configured", http.StatusNotFound)
		return
	}

	data := s.getStoreData(r)
	allEntities := data.finder.FindEntities(data.repo, "")

	reportedGroups := s.linter.ReportedGroups()
	isReported := func(owner string) bool {
		if len(reportedGroups) == 0 {
			return true // No restriction => all groups are reported
		}
		return slices.Contains(reportedGroups, owner)
	}

	// owner -> system -> []entityFindings
	grouped := make(map[string]map[string][]lintEntityFindings)

	for _, e := range allEntities {
		// Ignore Domain findings for now
		if e.GetKind() == catalog.KindDomain {
			continue
		}

		findings := s.getFindings(data, e)
		if len(findings) == 0 {
			continue
		}

		owner := "Unknown Owner"
		if o := e.GetOwner(); o != nil {
			owner = o.QName()
		}

		if !isReported(owner) {
			owner = "Others"
		}

		system := "No System"
		if sp, ok := e.(catalog.SystemPart); ok {
			if sysRef := sp.GetSystem(); sysRef != nil {
				system = sysRef.QName()
			}
		}

		if _, ok := grouped[owner]; !ok {
			grouped[owner] = make(map[string][]lintEntityFindings)
		}
		grouped[owner][system] = append(grouped[owner][system], lintEntityFindings{
			Entity:   e,
			Findings: findings,
		})
	}

	var result []ownerLintGroups
	for owner, systems := range grouped {
		olg := ownerLintGroups{Owner: owner}
		for system, entities := range systems {
			slices.SortFunc(entities, func(a, b lintEntityFindings) int {
				return strings.Compare(a.Entity.GetQName(), b.Entity.GetQName())
			})
			olg.Systems = append(olg.Systems, lintGroup{
				Name:     system,
				Entities: entities,
			})

			for _, ef := range entities {
				for _, f := range ef.Findings {
					switch f.Severity {
					case lint.SeverityError:
						olg.ErrorCount++
					case lint.SeverityWarn:
						olg.WarnCount++
					case lint.SeverityInfo:
						olg.InfoCount++
					}
				}
			}
		}
		slices.SortFunc(olg.Systems, func(a, b lintGroup) int {
			// "No System" should come last
			if a.Name == "No System" {
				return 1
			}
			if b.Name == "No System" {
				return -1
			}
			return strings.Compare(a.Name, b.Name)
		})
		result = append(result, olg)
	}

	slices.SortFunc(result, func(a, b ownerLintGroups) int {
		// "Others" should come last
		if a.Owner == "Others" {
			return 1
		}
		if b.Owner == "Others" {
			return -1
		}
		return strings.Compare(a.Owner, b.Owner)
	})

	var linkCheckSources []string
	if s.linkCheckEnabled() {
		linkCheckSources = s.linkFetchers().Names()
	}

	params := map[string]any{
		"PageTitle":        "Lint",
		"OwnerGroups":      result,
		"HasKube":          s.kubeScanEnabled(),
		"HasPrometheus":    s.prometheusScanEnabled(),
		"HasBitbucket":     s.bitbucketScanEnabled(),
		"LinkCheckSources": linkCheckSources,
	}

	s.serveHTMLPage(w, r, "lint_findings.html", params)
}

func (s *Server) serveKubeWorkloads(w http.ResponseWriter, r *http.Request) {
	data := s.getStoreData(r)
	untrackedOnly := r.URL.Query().Get("untracked") == "on"

	scan := s.scanKubeWorkloads(r.Context(), data, untrackedOnly)
	if !scan.Outcome.ok() {
		s.renderErrorSnippet(w, scan.Outcome.reason)
		return
	}

	params := map[string]any{
		"Workloads": scan.Workloads,
		"LabelKeys": []string{"app", "app.kubernetes.io/version"},
	}
	s.serveHTMLPage(w, r, "kube_workloads.html", params)
}

func (s *Server) servePrometheusWorkloads(w http.ResponseWriter, r *http.Request) {
	data := s.getStoreData(r)
	untrackedOnly := r.URL.Query().Get("untracked") == "on"

	scan := s.scanPrometheusWorkloads(r.Context(), data, untrackedOnly)
	if !scan.Outcome.ok() {
		s.renderErrorSnippet(w, scan.Outcome.reason)
		return
	}

	params := map[string]any{
		"Workloads":     scan.Workloads,
		"DisplayLabels": s.linter.Prometheus().DisplayLabels,
		"ShowMetrics":   s.linter.Prometheus().ShowMetrics,
	}
	s.serveHTMLPage(w, r, "prometheus_workloads.html", params)
}

// bitbucketResultView adds the display-only fields the scan itself has no
// business knowing about.
type bitbucketResultView struct {
	lint.BitbucketScanResult
	Tracked bool
	FileURL string
}

func (s *Server) serveBitbucketResults(w http.ResponseWriter, r *http.Request) {
	data := s.getStoreData(r)
	untrackedOnly := r.URL.Query().Get("untracked") == "on"
	refresh := r.URL.Query().Get("rescan") == "on"

	scan := s.scanBitbucketFiles(r.Context(), data, untrackedOnly, refresh)
	if !scan.Outcome.ok() {
		s.renderErrorSnippet(w, scan.Outcome.reason)
		return
	}

	views := make([]bitbucketResultView, 0, len(scan.Results))
	for _, res := range scan.Results {
		f := res.File
		views = append(views, bitbucketResultView{
			BitbucketScanResult: res,
			Tracked:             res.Entity != nil,
			FileURL:             fmt.Sprintf("%s/projects/%s/repos/%s/browse/%s", s.bbClient.BaseURL(), f.ProjectKey, f.RepoSlug, f.Path),
		})
	}

	params := map[string]any{
		"Results": views,
	}
	s.serveHTMLPage(w, r, "bitbucket_results.html", params)
}

func (s *Server) serveLinkCheckResults(w http.ResponseWriter, r *http.Request) {
	data := s.getStoreData(r)
	brokenOnly := r.URL.Query().Get("broken") == "on"

	scan := s.scanLinks(r.Context(), data, brokenOnly)
	if !scan.Outcome.ok() {
		s.renderErrorSnippet(w, scan.Outcome.reason)
		return
	}

	params := map[string]any{
		"Results": scan.Rows,
	}
	s.serveHTMLPage(w, r, "link_check_results.html", params)
}
