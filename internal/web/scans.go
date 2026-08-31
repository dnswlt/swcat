package web

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/kube"
	"github.com/dnswlt/swcat/internal/lint"
)

// The catalog-wide scans reach out to external systems, so they are shared
// between the lint UI and the findings API rather than implemented per surface:
// what counts as enabled, what counts as tracked, and how results are ordered
// must not depend on who is asking. Each surface still presents the result its
// own way -- the UI renders an error snippet where the API reports a status.

const (
	// The workload scans issue a single query; the Bitbucket and link scans fan
	// out over every entity, so they get considerably longer.
	workloadScanTimeout = 30 * time.Second
	deepScanTimeout     = 5 * time.Minute

	// bitbucketConcurrency caps how many Bitbucket requests run in parallel.
	// Kept conservative to avoid overloading the Bitbucket server.
	bitbucketConcurrency = 4
)

type scanState int

const (
	scanStateOK scanState = iota
	// The scan cannot run in this deployment. Distinct from a scan that ran
	// and found nothing.
	scanStateNotConfigured
	scanStateFailed
)

// scanOutcome is how a scan ended, independent of how it will be presented.
type scanOutcome struct {
	state scanState
	// reason explains a non-OK state in terms fit to show a user.
	reason string
}

func (o scanOutcome) ok() bool { return o.state == scanStateOK }

func scanSucceeded() scanOutcome { return scanOutcome{state: scanStateOK} }

func scanNotConfigured(reason string) scanOutcome {
	return scanOutcome{state: scanStateNotConfigured, reason: reason}
}

func scanFailed(format string, args ...any) scanOutcome {
	return scanOutcome{state: scanStateFailed, reason: fmt.Sprintf(format, args...)}
}

// Whether each scan can run at all. Every surface asks these rather than
// spelling out the condition, so a new prerequisite is added in one place.

func (s *Server) kubeScanEnabled() bool {
	return s.kubeClient != nil && s.linter != nil && s.linter.Kube().Enabled
}

func (s *Server) prometheusScanEnabled() bool {
	return s.promClient != nil && s.linter != nil && s.linter.Prometheus().Enabled
}

func (s *Server) bitbucketScanEnabled() bool {
	return s.bbClient != nil && s.linter != nil && s.linter.Bitbucket().Enabled
}

// linkCheckEnabled reports whether links can be checked, which needs a linter
// and at least one fetcher to do the fetching.
func (s *Server) linkCheckEnabled() bool {
	return s.linter != nil && len(s.linkFetchers().Names()) > 0
}

func (s *Server) linkFetchers() lint.LinkFetchers {
	return lint.LinkFetchers{Bitbucket: s.bbClient}
}

// --- Workload scans ---

// matchedKubeWorkload is a cluster workload together with the catalog entities
// that claim it. No matches means the catalog does not describe the workload.
type matchedKubeWorkload struct {
	kube.Workload
	Matched []*catalog.Ref
}

// Tracked reports whether any catalog entity claims this workload. Derived from
// Matched rather than computed separately, so the two cannot disagree.
func (w matchedKubeWorkload) Tracked() bool { return len(w.Matched) > 0 }

type kubeWorkloadScan struct {
	Outcome   scanOutcome
	Workloads []matchedKubeWorkload
}

func (s *Server) scanKubeWorkloads(ctx context.Context, data *storeData, untrackedOnly bool) kubeWorkloadScan {
	if !s.kubeScanEnabled() {
		return kubeWorkloadScan{Outcome: scanNotConfigured("Kubernetes workload scan not enabled")}
	}

	ctx, cancel := context.WithTimeout(ctx, workloadScanTimeout)
	defer cancel()

	all, err := s.kubeClient.AllWorkloads(ctx)
	if err != nil {
		log.Printf("Error fetching workloads: %v", err)
		return kubeWorkloadScan{Outcome: scanFailed("Error fetching workloads: %v", err)}
	}

	matcher := newWorkloadMatcher(data, catalog.AnnotKubeName)

	out := kubeWorkloadScan{Outcome: scanSucceeded()}
	for _, w := range all {
		if s.linter.IsExcludedKubeWorkload(w.Name) {
			continue
		}
		matched := matcher.match(w.Name)
		if untrackedOnly && len(matched) > 0 {
			continue
		}
		out.Workloads = append(out.Workloads, matchedKubeWorkload{Workload: w, Matched: matched})
	}

	slices.SortFunc(out.Workloads, func(a, b matchedKubeWorkload) int {
		if c := strings.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		if c := strings.Compare(string(a.Kind), string(b.Kind)); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// matchedPromWorkload is a workload Prometheus reports metrics for, together
// with the catalog entities that claim it.
type matchedPromWorkload struct {
	lint.PromWorkload
	Matched []*catalog.Ref
}

func (w matchedPromWorkload) Tracked() bool { return len(w.Matched) > 0 }

type promWorkloadScan struct {
	Outcome   scanOutcome
	Workloads []matchedPromWorkload
}

func (s *Server) scanPrometheusWorkloads(ctx context.Context, data *storeData, untrackedOnly bool) promWorkloadScan {
	if !s.prometheusScanEnabled() {
		return promWorkloadScan{Outcome: scanNotConfigured("Prometheus workload scan not enabled")}
	}

	ctx, cancel := context.WithTimeout(ctx, workloadScanTimeout)
	defer cancel()

	// Workloads excluded by configuration are dropped inside the scan itself.
	all, err := s.linter.ScanPrometheusWorkloads(ctx, s.promClient)
	if err != nil {
		log.Printf("Error scanning prometheus workloads: %v", err)
		return promWorkloadScan{Outcome: scanFailed("Error scanning prometheus workloads: %v", err)}
	}

	annotationKey := catalog.AnnotKubeName
	if a := s.linter.Prometheus().WorkloadNameAnnotation; a != "" {
		annotationKey = a
	}
	matcher := newWorkloadMatcher(data, annotationKey)

	out := promWorkloadScan{Outcome: scanSucceeded()}
	for _, w := range all {
		matched := matcher.match(w.Name)
		if untrackedOnly && len(matched) > 0 {
			continue
		}
		out.Workloads = append(out.Workloads, matchedPromWorkload{PromWorkload: w, Matched: matched})
	}

	slices.SortFunc(out.Workloads, func(a, b matchedPromWorkload) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// workloadMatcher resolves a workload name to the catalog entities claiming it,
// either by being a component of that name or by carrying the name in an
// annotation. The annotation index is built once per scan, not per workload.
type workloadMatcher struct {
	data         *storeData
	byAnnotation map[string][]*catalog.Ref
}

func newWorkloadMatcher(data *storeData, annotationKey string) *workloadMatcher {
	m := &workloadMatcher{
		data:         data,
		byAnnotation: make(map[string][]*catalog.Ref),
	}
	for _, c := range data.finder.FindComponents(data.repo, "") {
		if v, ok := c.GetMetadata().Annotations[annotationKey]; ok && v != "" {
			m.byAnnotation[v] = append(m.byAnnotation[v], c.GetRef())
		}
	}
	return m
}

// match returns the entities claiming the named workload, sorted and
// deduplicated. Empty means untracked. More than one entry means several
// entities claim the same workload, which the annotation index permits and
// which is worth surfacing rather than resolving arbitrarily.
func (m *workloadMatcher) match(name string) []*catalog.Ref {
	var refs []*catalog.Ref
	byName := &catalog.Ref{
		Kind:      catalog.KindComponent,
		Namespace: catalog.DefaultNamespace,
		Name:      name,
	}
	if c := m.data.repo.Component(byName); c != nil {
		refs = append(refs, c.GetRef())
	}
	refs = append(refs, m.byAnnotation[name]...)

	slices.SortFunc(refs, func(a, b *catalog.Ref) int {
		return strings.Compare(a.String(), b.String())
	})
	return slices.CompactFunc(refs, func(a, b *catalog.Ref) bool {
		return a.String() == b.String()
	})
}

// --- Bitbucket file scan ---

type bitbucketFileScan struct {
	Outcome scanOutcome
	Results []lint.BitbucketScanResult
}

func (s *Server) scanBitbucketFiles(ctx context.Context, data *storeData, untrackedOnly, refresh bool) bitbucketFileScan {
	if !s.bitbucketScanEnabled() {
		return bitbucketFileScan{Outcome: scanNotConfigured("Bitbucket scan not enabled")}
	}

	ctx, cancel := context.WithTimeout(ctx, deepScanTimeout)
	defer cancel()

	useCache := !refresh
	log.Printf("Looking for files in Bitbucket (useCache=%v)", useCache)
	files, err := s.linter.FindBitbucketFiles(ctx, s.bbClient, useCache, bitbucketConcurrency)
	if err != nil {
		log.Printf("Error scanning Bitbucket files: %v", err)
		return bitbucketFileScan{Outcome: scanFailed("Error scanning Bitbucket files: %v", err)}
	}
	log.Printf("Found %d files. Matching files against entity URLs.", len(files))

	results := s.linter.MatchBitbucketFiles(files, data.finder.FindEntities(data.repo, ""))
	if untrackedOnly {
		results = slices.DeleteFunc(results, func(r lint.BitbucketScanResult) bool {
			return r.Entity != nil
		})
	}
	slices.SortFunc(results, func(a, b lint.BitbucketScanResult) int {
		if c := strings.Compare(a.File.ProjectKey, b.File.ProjectKey); c != 0 {
			return c
		}
		if c := strings.Compare(a.File.RepoSlug, b.File.RepoSlug); c != 0 {
			return c
		}
		return strings.Compare(a.File.Path, b.File.Path)
	})
	return bitbucketFileScan{Outcome: scanSucceeded(), Results: results}
}

// --- Link check ---

// linkCheckRow is one checked link belonging to one entity.
type linkCheckRow struct {
	Entity catalog.Entity
	URL    string
	Title  string
	Type   string
	// Status is one of "ok", "broken", "warn". The UI template picks an icon
	// and colour from it; the API reports it verbatim.
	Status string
	Reason string
}

type linkCheckScan struct {
	Outcome scanOutcome
	Rows    []linkCheckRow
}

func (s *Server) scanLinks(ctx context.Context, data *storeData, brokenOnly bool) linkCheckScan {
	if s.linter == nil {
		return linkCheckScan{Outcome: scanNotConfigured("Linter not configured")}
	}
	if !s.linkCheckEnabled() {
		return linkCheckScan{Outcome: scanNotConfigured("No link fetchers configured")}
	}

	ctx, cancel := context.WithTimeout(ctx, deepScanTimeout)
	defer cancel()

	entities := data.finder.FindEntities(data.repo, "")
	out := linkCheckScan{Outcome: scanSucceeded()}
	for _, c := range s.linter.ScanLinks(ctx, s.linkFetchers(), entities, bitbucketConcurrency) {
		status, reason := linkCheckOutcome(c.Result)
		// "Broken only" hides everything except confirmed-broken links.
		if brokenOnly && status != "broken" {
			continue
		}
		out.Rows = append(out.Rows, linkCheckRow{
			Entity: c.Entity,
			URL:    c.Link.URL,
			Title:  c.Link.Title,
			Type:   c.Link.Type,
			Status: status,
			Reason: reason,
		})
	}

	slices.SortFunc(out.Rows, func(a, b linkCheckRow) int {
		if c := strings.Compare(a.Entity.GetQName(), b.Entity.GetQName()); c != 0 {
			return c
		}
		return strings.Compare(a.URL, b.URL)
	})
	return out
}

// linkCheckOutcome renders a link check result as the status string used by both
// surfaces ("ok", "broken", or "warn" when the link could be confirmed neither
// way), together with a human-readable reason.
func linkCheckOutcome(res lint.LinkCheckResult) (status, reason string) {
	switch res.Status {
	case lint.LinkCheckOK:
		status = "ok"
	case lint.LinkCheckBroken:
		status = "broken"
	default:
		// LinkCheckError, LinkCheckNoAccess: cannot be confirmed either way.
		status = "warn"
	}
	reason = res.Reason
	if res.Err != nil {
		if reason != "" {
			reason = fmt.Sprintf("%s: %v", reason, res.Err)
		} else {
			reason = res.Err.Error()
		}
	}
	return status, reason
}
