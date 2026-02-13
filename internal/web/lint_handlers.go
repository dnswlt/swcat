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
		"OwnerGroups": result,
		"HasKube":     s.kubeClient != nil,
	}

	s.serveHTMLPage(w, r, "lint_findings.html", params)
}

type kubeWorkloadView struct {
	kube.Workload
	Tracked bool
}

func (s *Server) serveKubeWorkloads(w http.ResponseWriter, r *http.Request) {
	if s.kubeClient == nil {
		http.Error(w, "Kubernetes client not configured", http.StatusNotFound)
		return
	}

	data := s.getStoreData(r)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	allWorkloads, err := s.kubeClient.AllWorkloads(ctx)
	if err != nil {
		log.Printf("Error fetching workloads: %v", err)
		http.Error(w, fmt.Sprintf("Error fetching workloads: %v", err), http.StatusInternalServerError)
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

	var workloads []kubeWorkloadView
	for _, w := range allWorkloads {
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
