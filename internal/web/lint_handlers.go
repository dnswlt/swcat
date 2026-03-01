package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/kube"
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
	allEntities := s.finder.FindEntities(data.repo, "")

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

	params := map[string]any{
		"OwnerGroups":   result,
		"HasKube":       s.kubeClient != nil && s.linter != nil && s.linter.Kube().Enabled,
		"HasPrometheus": s.promClient != nil && s.linter != nil && s.linter.Prometheus().Enabled,
		"HasBitbucket":  s.bbClient != nil && s.linter != nil && s.linter.Bitbucket().Enabled,
	}

	s.serveHTMLPage(w, r, "lint_findings.html", params)
}

type kubeWorkloadView struct {
	kube.Workload
	Tracked bool
}

func (s *Server) serveKubeWorkloads(w http.ResponseWriter, r *http.Request) {
	if s.kubeClient == nil || s.linter == nil || !s.linter.Kube().Enabled {
		s.renderErrorSnippet(w, "Kubernetes workload scan not enabled")
		return
	}

	data := s.getStoreData(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	allWorkloads, err := s.kubeClient.AllWorkloads(ctx)
	if err != nil {
		log.Printf("Error fetching workloads: %v", err)
		s.renderErrorSnippet(w, fmt.Sprintf("Error fetching workloads: %v", err))
		return
	}

	// Build a set of known workload names from annotations.
	annotatedNames := make(map[string]bool)
	for _, e := range s.finder.FindComponents(data.repo, "") {
		if v, ok := e.GetMetadata().Annotations[catalog.AnnotKubeName]; ok {
			annotatedNames[v] = true
		}
	}

	isTracked := func(w kube.Workload) bool {
		// Tracked if a Component with the same name exists in the default namespace.
		ref := &catalog.Ref{Kind: catalog.KindComponent, Namespace: catalog.DefaultNamespace, Name: w.Name}
		if data.repo.Component(ref) != nil {
			return true
		}
		// Tracked if any entity has a matching annotation.
		return annotatedNames[w.Name]
	}

	excluded := make(map[string]bool)
	if s.linter != nil {
		for _, name := range s.linter.Kube().ExcludedWorkloads {
			excluded[name] = true
		}
	}

	var workloads []kubeWorkloadView
	for _, w := range allWorkloads {
		if excluded[w.Name] {
			continue
		}
		workloads = append(workloads, kubeWorkloadView{
			Workload: w,
			Tracked:  isTracked(w),
		})
	}

	// Filter to untracked workloads if requested.
	untrackedOnly := r.URL.Query().Get("untracked") == "on"
	if untrackedOnly {
		workloads = slices.DeleteFunc(workloads, func(w kubeWorkloadView) bool {
			return w.Tracked
		})
	}

	slices.SortFunc(workloads, func(a, b kubeWorkloadView) int {
		if c := strings.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		if c := strings.Compare(string(a.Kind), string(b.Kind)); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})

	params := map[string]any{
		"Workloads": workloads,
		"LabelKeys": []string{"app", "app.kubernetes.io/version"},
	}
	s.serveHTMLPage(w, r, "kube_workloads.html", params)
}

type prometheusWorkloadView struct {
	lint.PromWorkload
	Tracked bool
}

func (s *Server) servePrometheusWorkloads(w http.ResponseWriter, r *http.Request) {
	if s.promClient == nil || s.linter == nil || !s.linter.Prometheus().Enabled {
		s.renderErrorSnippet(w, "Prometheus workload scan not enabled")
		return
	}

	data := s.getStoreData(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	promWorkloads, err := s.linter.ScanPrometheusWorkloads(ctx, s.promClient)
	if err != nil {
		log.Printf("Error scanning prometheus workloads: %v", err)
		s.renderErrorSnippet(w, fmt.Sprintf("Error scanning prometheus workloads: %v", err))
		return
	}

	// Build a set of known workload names from annotations.
	annotatedNames := make(map[string]bool)
	annotationKey := catalog.AnnotKubeName
	if a := s.linter.Prometheus().WorkloadNameAnnotation; a != "" {
		annotationKey = a
	}
	for _, e := range s.finder.FindComponents(data.repo, "") {
		if v, ok := e.GetMetadata().Annotations[annotationKey]; ok {
			annotatedNames[v] = true
		}
	}

	isTracked := func(w lint.PromWorkload) bool {
		// Tracked if a Component with the same name exists in the default namespace.
		ref := &catalog.Ref{Kind: catalog.KindComponent, Namespace: catalog.DefaultNamespace, Name: w.Name}
		if data.repo.Component(ref) != nil {
			return true
		}
		// Tracked if any entity has a matching annotation.
		return annotatedNames[w.Name]
	}

	var workloads []prometheusWorkloadView
	for _, w := range promWorkloads {
		workloads = append(workloads, prometheusWorkloadView{
			PromWorkload: w,
			Tracked:      isTracked(w),
		})
	}

	// Filter to untracked workloads if requested.
	untrackedOnly := r.URL.Query().Get("untracked") == "on"
	if untrackedOnly {
		workloads = slices.DeleteFunc(workloads, func(w prometheusWorkloadView) bool {
			return w.Tracked
		})
	}

	slices.SortFunc(workloads, func(a, b prometheusWorkloadView) int {
		return strings.Compare(a.Name, b.Name)
	})

	params := map[string]any{
		"Workloads":     workloads,
		"DisplayLabels": s.linter.Prometheus().DisplayLabels,
		"ShowMetrics":   s.linter.Prometheus().ShowMetrics,
	}
	s.serveHTMLPage(w, r, "prometheus_workloads.html", params)
}

type bitbucketResultView struct {
	lint.BitbucketScanResult
	Tracked bool
	FileURL string
}

func (s *Server) serveBitbucketResults(w http.ResponseWriter, r *http.Request) {
	if s.bbClient == nil || s.linter == nil || !s.linter.Bitbucket().Enabled {
		s.renderErrorSnippet(w, "Bitbucket scan not enabled")
		return
	}

	data := s.getStoreData(r)
	entities := s.finder.FindEntities(data.repo, "")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	useCache := r.URL.Query().Get("rescan") != "on"
	log.Printf("Looking for files in Bitbucket (useCache=%v)", useCache)
	queryResults := s.linter.FindBitbucketFiles(ctx, s.bbClient, useCache)
	log.Printf("Found %d files. Matching files against entity URLs.", len(queryResults))
	scanResults := s.linter.MatchBitbucketFiles(queryResults, entities)

	untrackedOnly := r.URL.Query().Get("untracked") == "on"
	var views []bitbucketResultView
	for _, res := range scanResults {
		tracked := res.Entity != nil
		if untrackedOnly && tracked {
			continue
		}
		f := res.File
		views = append(views, bitbucketResultView{
			BitbucketScanResult: res,
			Tracked:             tracked,
			FileURL:             fmt.Sprintf("%s/projects/%s/repos/%s/browse/%s", s.bbClient.BaseURL(), f.ProjectKey, f.RepoSlug, f.Path),
		})
	}

	slices.SortFunc(views, func(a, b bitbucketResultView) int {
		if c := strings.Compare(a.File.ProjectKey, b.File.ProjectKey); c != 0 {
			return c
		}
		if c := strings.Compare(a.File.RepoSlug, b.File.RepoSlug); c != 0 {
			return c
		}
		return strings.Compare(a.File.Path, b.File.Path)
	})

	params := map[string]any{
		"Results": views,
	}
	s.serveHTMLPage(w, r, "bitbucket_results.html", params)
}
