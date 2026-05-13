package web

import (
	"maps"
	"slices"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
)

// FullyConnectedGraph returns all entities on any simple (cycle-free) directed
// path connecting two entities in roots, including the roots themselves.
//
// A path is defined by the following edges:
// (a) Component -(consumesApis)-> API
// (b) API -(providedBy)-> Component
// (c) (Resource|Component) -(dependsOn)-> (Component|Resource)
//
// The algorithm uses a Depth-First Search (DFS) from each root to identify
// all nodes that can reach a root (including themselves) without forming a cycle.
func FullyConnectedGraph(r *repo.Repository, roots []catalog.Entity) []catalog.Entity {
	if len(roots) == 0 {
		return nil
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, e := range roots {
		rootSet[e.GetRef().String()] = struct{}{}
	}

	// result stores the entities on simple paths between roots.
	result := make(map[string]catalog.Entity)
	// pathSet tracks the current recursion stack to identify cycles.
	pathSet := make(map[string]struct{})

	var dfs func(e catalog.Entity) bool
	dfs = func(e catalog.Entity) bool {
		key := e.GetRef().String()
		if _, ok := pathSet[key]; ok {
			return false
		}

		pathSet[key] = struct{}{}
		defer delete(pathSet, key)

		// A node is on a valid path if it's a root or if it can reach one.
		_, found := rootSet[key]
		for _, nb := range forwardNeighbors(r, e) {
			if dfs(nb) {
				found = true
			}
		}

		if found {
			result[key] = e
		}
		return found
	}

	for _, root := range roots {
		dfs(root)
	}

	return slices.Collect(maps.Values(result))
}

// forwardNeighbors returns the entities that the given entity has a directed edge to,
// according to the rules defined in FullyConnectedGraph.
func forwardNeighbors(r *repo.Repository, e catalog.Entity) []catalog.Entity {
	var out []catalog.Entity

	addEntities := func(refs []*catalog.LabelRef) {
		for _, lr := range refs {
			if nb := r.Entity(lr.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	}

	switch v := e.(type) {
	case *catalog.Component:
		addEntities(v.Spec.ConsumesAPIs)
		addEntities(v.Spec.DependsOn)
	case *catalog.API:
		addEntities(v.GetProviders())
	case *catalog.Resource:
		addEntities(v.Spec.DependsOn)
	}
	return out
}
