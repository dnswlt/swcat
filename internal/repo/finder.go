package repo

import (
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
)

// Finder is responsible for searching the repository.
// It holds any registered property providers.
type Finder struct {
	providers []query.PropertyProvider
}

// NewFinder creates a new Finder.
func NewFinder(providers ...query.PropertyProvider) *Finder {
	return &Finder{
		providers: providers,
	}
}

// RegisterPropertyProvider adds a new property provider to the finder.
func (f *Finder) RegisterPropertyProvider(p query.PropertyProvider) {
	f.providers = append(f.providers, p)
}

func findEntities[T catalog.Entity](q string, items map[string]T, providers []query.PropertyProvider) []T {
	var result []T

	if strings.TrimSpace(q) == "" {
		// No filter, return all items
		result = make([]T, 0, len(items))
		for _, item := range items {
			result = append(result, item)
		}
	} else {
		expr, err := query.Parse(q)
		if err != nil {
			return nil // Invalid query => no results
		}
		ev := query.NewEvaluator(expr, providers...)
		for _, c := range items {
			ok, err := ev.Matches(c)
			if err != nil {
				return nil // Broken query (e.g. broken regex) => no results
			}
			if ok {
				result = append(result, c)
			}
		}
	}
	slices.SortFunc(result, func(c1, c2 T) int {
		return catalog.CompareEntityByRef(c1, c2)
	})
	return result
}

func (f *Finder) FindComponents(repo *Repository, q string) []*catalog.Component {
	return findEntities(q, repo.components, f.providers)
}

func (f *Finder) FindSystems(repo *Repository, q string) []*catalog.System {
	return findEntities(q, repo.systems, f.providers)
}

func (f *Finder) FindAPIs(repo *Repository, q string) []*catalog.API {
	return findEntities(q, repo.apis, f.providers)
}

func (f *Finder) FindResources(repo *Repository, q string) []*catalog.Resource {
	return findEntities(q, repo.resources, f.providers)
}

func (f *Finder) FindDomains(repo *Repository, q string) []*catalog.Domain {
	return findEntities(q, repo.domains, f.providers)
}

func (f *Finder) FindGroups(repo *Repository, q string) []*catalog.Group {
	return findEntities(q, repo.groups, f.providers)
}

func (f *Finder) FindEntities(repo *Repository, q string) []catalog.Entity {
	return findEntities(q, repo.allEntities, f.providers)
}
