package web

import (
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
	}

	s.serveHTMLPage(w, r, "lint_findings.html", params)
}
