package svg

import (
	"context"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
)

// flightsRepo loads the bundled example, which is the only fixture with several
// domains related to each other.
func flightsRepo(t *testing.T) *repo.Repository {
	t.Helper()
	r, err := repo.Load(store.NewDiskStore("../../examples/flights"), nil, repo.Config{})
	if err != nil {
		t.Fatalf("failed to load the flights example: %v", err)
	}
	return r
}

// domainView renders a domain's external view and returns what it shows.
func domainView(t *testing.T, opts *DomainViewOptions) (nodes, edges map[string]bool) {
	t.Helper()
	r := flightsRepo(t)
	dom := r.Domain(&catalog.Ref{Name: "flights"})
	if dom == nil {
		t.Fatal("domain flights not found")
	}
	res, err := NewRenderer(r, nil, DefaultConfig()).
		DomainExternalGraph(context.Background(), dom, opts)
	if err != nil {
		t.Fatalf("DomainExternalGraph failed: %v", err)
	}
	nodes = map[string]bool{}
	for id := range res.Metadata.Nodes {
		nodes[id] = true
	}
	edges = map[string]bool{}
	for _, e := range res.Metadata.Edges {
		edges[e.From+" -> "+e.To] = true
	}
	return nodes, edges
}

// At the systems level a neighboring domain is a frame with its systems inside,
// and the relationships are between systems.
func TestDomainExternalGraph_Systems(t *testing.T) {
	nodes, edges := domainView(t, NewDomainViewOptions(nil, DetailSystems))

	for _, want := range []string{"system:iam-system", "system:analytics-pipeline"} {
		if !nodes[want] {
			t.Errorf("missing node %q; got %v", want, nodes)
		}
	}
	if nodes["domain:iam"] {
		t.Errorf("neighbors should be systems at this level; got %v", nodes)
	}
	if !edges["system:flights-tickets -> system:iam-system"] {
		t.Errorf("missing edge flights-tickets -> iam-system; got %v", edges)
	}
	// Relationships inside the domain belong to its internal view.
	for e := range edges {
		if strings.Contains(e, "flights-search -> system:flights-tickets") {
			t.Errorf("intra-domain edge %q leaked into the external view", e)
		}
	}
}

// At the domains level a neighbor is one box, and the systems behind it are
// represented by their domain.
func TestDomainExternalGraph_Domains(t *testing.T) {
	nodes, edges := domainView(t, NewDomainViewOptions(nil, DetailDomains))

	if !nodes["domain:iam"] {
		t.Errorf("missing node domain:iam; got %v", nodes)
	}
	if nodes["system:iam-system"] {
		t.Errorf("neighbor systems should be collapsed; got %v", nodes)
	}
	if !edges["system:flights-tickets -> domain:iam"] {
		t.Errorf("missing edge flights-tickets -> domain:iam; got %v", edges)
	}
	// The focal domain's own systems stay visible: they are what the
	// relationships start from.
	if !nodes["system:flights-tickets"] {
		t.Errorf("the domain's own systems should still be drawn; got %v", nodes)
	}
}

func TestDomainExternalGraph_SelectionNarrows(t *testing.T) {
	iam := &catalog.Ref{Kind: catalog.KindDomain, Name: "iam"}
	nodes, _ := domainView(t, NewDomainViewOptions([]*catalog.Ref{iam}, DetailSystems))

	if !nodes["system:iam-system"] {
		t.Errorf("the selected domain should be shown; got %v", nodes)
	}
	if nodes["system:analytics-pipeline"] {
		t.Errorf("unselected domains should be left out; got %v", nodes)
	}
}

// The internal view is the other half: the domain's own systems and how they
// relate to each other, and nothing from outside.
func TestDomainInternalGraph(t *testing.T) {
	r := flightsRepo(t)
	dom := r.Domain(&catalog.Ref{Name: "flights"})
	runner := &mockRunner{}
	if _, err := NewRenderer(r, runner, DefaultConfig()).
		DomainInternalGraph(context.Background(), dom); err != nil {
		t.Fatalf("DomainInternalGraph failed: %v", err)
	}

	src := runner.lastDotSource
	if !strings.Contains(src, `"system:flights-search" -> "system:flights-tickets"`) {
		t.Errorf("missing intra-domain edge; got:\n%s", src)
	}
	for _, outside := range []string{"system:iam-system", "system:analytics-pipeline"} {
		if strings.Contains(src, outside) {
			t.Errorf("%s is outside the domain and should not appear", outside)
		}
	}
}

// Relationships are gathered through maps, so the collected view has to be put
// back into a defined order — otherwise the same page renders differently on
// every request.
func TestDomainExternalGraphIsDeterministic(t *testing.T) {
	r := flightsRepo(t)
	dom := r.Domain(&catalog.Ref{Name: "flights"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	var first string
	for i := range 8 {
		res, err := renderer.DomainExternalGraph(context.Background(), dom,
			NewDomainViewOptions(nil, DetailSystems))
		if err != nil {
			t.Fatalf("DomainExternalGraph failed: %v", err)
		}
		if i == 0 {
			first = string(res.SVG)
			continue
		}
		if string(res.SVG) != first {
			t.Fatal("the same domain view rendered differently on a second call")
		}
	}
}

func TestDomainInternalGraphIsDeterministic(t *testing.T) {
	r := flightsRepo(t)
	dom := r.Domain(&catalog.Ref{Name: "flights"})
	runner := &mockRunner{}
	renderer := NewRenderer(r, runner, DefaultConfig())

	var first string
	for i := range 8 {
		if _, err := renderer.DomainInternalGraph(context.Background(), dom); err != nil {
			t.Fatalf("DomainInternalGraph failed: %v", err)
		}
		if i == 0 {
			first = runner.lastDotSource
			continue
		}
		if runner.lastDotSource != first {
			t.Fatal("the same domain view produced different dot source on a second call")
		}
	}
}

// At the domains level one arrow stands for every system behind a neighboring
// domain, rather than one arrow per system.
func TestDomainArrowsAggregatePerBoxPair(t *testing.T) {
	r := flightsRepo(t)
	dom := r.Domain(&catalog.Ref{Name: "flights"})
	res, err := NewRenderer(r, nil, DefaultConfig()).
		DomainExternalGraph(context.Background(), dom, NewDomainViewOptions(nil, DetailDomains))
	if err != nil {
		t.Fatalf("DomainExternalGraph failed: %v", err)
	}

	pairs := map[string]int{}
	for _, e := range res.Metadata.Edges {
		pairs[e.From+" -> "+e.To]++
	}
	for pair, n := range pairs {
		if n > 1 {
			t.Errorf("%s is drawn %d times, want once", pair, n)
		}
	}
	// The payments domain holds two systems, both reached, but it is one arrow.
	if n := pairs["system:flights-tickets -> domain:payments"]; n != 1 {
		t.Errorf("arrow to payments drawn %d times, want once", n)
	}
}
