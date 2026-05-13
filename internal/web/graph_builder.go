package web

import (
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
)

// FullyConnectedGraph returns all entities that lie on any path
// connecting any two entities in roots, including roots.
// A path is defined by the following edges:
// (a) Component -(consumesApis)-> API
// (b) API -(providedBy)-> Component
// (c) (Resource|Component) -(dependsOn)-> (Component|Resource)
//
// The function uses two passes of multi-BFS starting from all entities
// in roots: one forward pass, one backward pass. It returns
// the intersection of the two passes.
func FullyConnectedGraph(r *repo.Repository, roots []catalog.Entity) []catalog.Entity {
	forward := bfsReachable(r, roots, forwardNeighbors)
	backward := bfsReachable(r, roots, backwardNeighbors)
	result := make([]catalog.Entity, 0, len(forward))
	for k, e := range forward {
		if _, ok := backward[k]; ok {
			result = append(result, e)
		}
	}
	return result
}

func bfsReachable(
	r *repo.Repository,
	roots []catalog.Entity,
	neighbors func(*repo.Repository, catalog.Entity) []catalog.Entity,
) map[string]catalog.Entity {
	visited := make(map[string]catalog.Entity)
	var queue []catalog.Entity
	for _, e := range roots {
		k := e.GetRef().String()
		if _, ok := visited[k]; !ok {
			visited[k] = e
			queue = append(queue, e)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range neighbors(r, cur) {
			k := nb.GetRef().String()
			if _, ok := visited[k]; !ok {
				visited[k] = nb
				queue = append(queue, nb)
			}
		}
	}
	return visited
}

func forwardNeighbors(r *repo.Repository, e catalog.Entity) []catalog.Entity {
	var out []catalog.Entity
	switch v := e.(type) {
	case *catalog.Component:
		for _, ref := range v.Spec.ConsumesAPIs {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
		for _, ref := range v.Spec.DependsOn {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	case *catalog.API:
		for _, ref := range v.GetProviders() {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	case *catalog.Resource:
		for _, ref := range v.Spec.DependsOn {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	}
	return out
}

func backwardNeighbors(r *repo.Repository, e catalog.Entity) []catalog.Entity {
	var out []catalog.Entity
	switch v := e.(type) {
	case *catalog.Component:
		for _, ref := range v.Spec.ProvidesAPIs {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
		for _, ref := range v.GetDependents() {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	case *catalog.API:
		for _, ref := range v.GetConsumers() {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	case *catalog.Resource:
		for _, ref := range v.GetDependents() {
			if nb := r.Entity(ref.Ref); nb != nil {
				out = append(out, nb)
			}
		}
	}
	return out
}
