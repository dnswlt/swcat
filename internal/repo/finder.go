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

// findEntities returns the items matching q, sorted by ref. An empty query
// matches all items. An invalid query (e.g. a broken regex or an unknown
// attribute) returns an error instead of results, whatever the catalog holds.
func findEntities[T catalog.Entity](repo *Repository, q string, items map[string]T, providers []query.PropertyProvider) ([]T, error) {
	var result []T

	if strings.TrimSpace(q) == "" {
		// No filter, return all items
		result = make([]T, 0, len(items))
		for _, item := range items {
			result = append(result, item)
		}
	} else {
		ev, err := query.Compile(q, providers...)
		if err != nil {
			return nil, err
		}
		for _, c := range items {
			if ev.Matches(c, repo) {
				result = append(result, c)
			}
		}
	}
	slices.SortFunc(result, func(c1, c2 T) int {
		return catalog.CompareEntityByRef(c1, c2)
	})
	return result, nil
}

func (f *Finder) FindComponents(repo *Repository, q string) ([]*catalog.Component, error) {
	return findEntities(repo, q, repo.components, f.providers)
}

func (f *Finder) FindSystems(repo *Repository, q string) ([]*catalog.System, error) {
	return findEntities(repo, q, repo.systems, f.providers)
}

func (f *Finder) FindAPIs(repo *Repository, q string) ([]*catalog.API, error) {
	return findEntities(repo, q, repo.apis, f.providers)
}

func (f *Finder) FindResources(repo *Repository, q string) ([]*catalog.Resource, error) {
	return findEntities(repo, q, repo.resources, f.providers)
}

func (f *Finder) FindDomains(repo *Repository, q string) ([]*catalog.Domain, error) {
	return findEntities(repo, q, repo.domains, f.providers)
}

func (f *Finder) FindGroups(repo *Repository, q string) ([]*catalog.Group, error) {
	return findEntities(repo, q, repo.groups, f.providers)
}

func (f *Finder) FindEntities(repo *Repository, q string) ([]catalog.Entity, error) {
	return findEntities(repo, q, repo.allEntities, f.providers)
}
