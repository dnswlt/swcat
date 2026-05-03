package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"github.com/dnswlt/swcat/internal/repo"
	"gopkg.in/yaml.v3"
)

const (
	// The status field in which the plugin stores its results.
	JFrogXrayPluginTarget = "swcat-plugins/jfrog-xray-sbom"
	// The status field that lint findings get written to.
	JFrogXrayPluginLintTarget = "swcat-lint/finding-jfrog-xray"
	// Annotation set on an entity to exclude it from JFrog Xray lint checks.
	// When the value is "true", the entity is dropped from the catalog index
	// used for linting, so it will never be reported as a missing dependency
	// for any other entity. This is the usual "lint:ignore" hook needed for
	// weird edge cases, to avoid flooding the system with lint warnings that
	// nobody looks at.
	JFrogXrayPluginLintIgnoreAnnotation = "swcat-lint/ignore-jfrog-xray"
	// Annotation in which to find the Docker image name for an entity.
	JFrogXrayPluginImageAnnotation = catalog.AnnotDockerImage
	// Annotation in which to find the Maven GAV coordinates for an entity.
	JFrogXrayPluginCoordsAnnotation = catalog.AnnotMavenCoords
)

type jfrogXrayPluginSpec struct {
	// ComponentsFilter defines which items of an SBOM to include in the MiniBOM.
	ComponentsFilter sbom.ComponentsFilter `yaml:"componentsFilter"`

	// If true, the plugin will detect missing dependencies and store them
	// in the entity's state.
	LintMissingDependencies bool `yaml:"lintMissingDependencies"`
}

// JFrogXrayClient is a client-side interface for a JFrog client that is used to fetch Xray data.
// A jfrog.Client should implement this interface.
type JFrogXrayClient interface {
	// https://{jfrog_url}/artifactory/api/docker/{repo-key}/v2/{imageName}/tags/list
	ListDockerTags(ctx context.Context, repository, image string) ([]string, error)
	// https://{jfrog_url}/xray/api/v2/component/exportDetails
	XrayExportDetails(ctx context.Context, repository, image, version string) ([]byte, error)
}

type JFrogXrayPlugin struct {
	name   string
	spec   *jfrogXrayPluginSpec
	client JFrogXrayClient

	// catalogIndex is memoized per repository pointer. Repositories are
	// immutable (InsertOrUpdateEntity returns a new instance), so pointer
	// identity is a precise cache key. Only one index is held at a time —
	// when a new repo arrives, the old one becomes GC-able.
	indexMu     sync.Mutex
	indexedRepo *repo.Repository
	index       *catalogIndex
}

// getCatalogIndex returns a catalogIndex for r, building it on the first call
// for a given repository instance and reusing it on subsequent calls.
func (p *JFrogXrayPlugin) getCatalogIndex(r *repo.Repository) *catalogIndex {
	p.indexMu.Lock()
	defer p.indexMu.Unlock()
	if p.indexedRepo == r {
		return p.index
	}
	p.index = newCatalogIndexFromEntities(r.AllEntities())
	p.indexedRepo = r
	return p.index
}

func NewJFrogXrayBOMPlugin(name string, specYaml *yaml.Node, client JFrogXrayClient) (*JFrogXrayPlugin, error) {
	var spec jfrogXrayPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode JFrogXrayPlugin spec for %s: %v", name, err)
	}

	if client == nil {
		return nil, fmt.Errorf("no JFrogXrayClient provided: %w", ErrPreconditionFailed)
	}
	return &JFrogXrayPlugin{
		name:   name,
		spec:   &spec,
		client: client,
	}, nil
}

type TagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type SBOMRequest struct {
	PackageType     string `json:"package_type"`
	ComponentName   string `json:"component_name"`
	Path            string `json:"path"`
	CycloneDX       bool   `json:"cyclonedx"`
	CycloneDXFormat string `json:"cyclonedx_format"`
	Vex             bool   `json:"vex"`
}

// latestVersions returns the three most recent semver Docker tags for image
// in repository, newest first.
func (p *JFrogXrayPlugin) latestVersions(ctx context.Context, repository, image string) ([]string, error) {
	tags, err := p.client.ListDockerTags(ctx, repository, image)
	if err != nil {
		return nil, err
	}
	versions := latestSemverVersions(tags, 3)
	if len(versions) == 0 {
		return nil, fmt.Errorf("no valid semver tags found for %s/%s", repository, image)
	}
	return versions, nil
}

// fetchSBOM retrieves the SBOM for the first version in versions whose Xray
// export succeeds, falling back in order. If it encounters prevVersion before
// a successful download, it returns a nil byte slice to indicate no update is needed.
func (p *JFrogXrayPlugin) fetchSBOM(ctx context.Context, repository, image string, versions []string, prevVersion string) ([]byte, string, error) {
	var lastErr error
	for _, version := range versions {
		if version == prevVersion {
			return nil, version, nil
		}
		data, err := p.client.XrayExportDetails(ctx, repository, image, version)
		if err != nil {
			log.Printf("fetchSBOM: skipping %s:%s: %v", image, version, err)
			lastErr = err
			continue
		}
		return data, version, nil
	}
	return nil, "", fmt.Errorf("could not download SBOM for %s/%s (tried %v): %w", repository, image, versions, lastErr)
}

func (p *JFrogXrayPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {

	repository, ok := args.Repository.IAnnotation(entity, JFrogDockerRepositoryAnnotation)
	if !ok || repository == "" {
		return nil, fmt.Errorf("No repository specified in annotation %q for %v", JFrogDockerRepositoryAnnotation, entity.GetQName())
	}

	image := entity.GetMetadata().Name
	if img, ok := entity.GetMetadata().Annotations[JFrogXrayPluginImageAnnotation]; ok {
		// Override image name from annotation
		image = img
	}

	// Fetch CycloneDX SBOM from jFrog XRay
	versions, err := p.latestVersions(ctx, repository, image)
	if err != nil {
		return nil, err
	}

	var prevVersion string
	if status := entity.GetStatus(); status != nil {
		if prev, ok := status.Observations[JFrogXrayPluginTarget]; ok {
			prevVersion = prev.Version
		}
	}

	sbomBytes, version, err := p.fetchSBOM(ctx, repository, image, versions, prevVersion)
	if err != nil {
		return nil, err
	}
	bomChanged := sbomBytes != nil

	// When the BOM is unchanged and lint is disabled, there's nothing to do.
	if !bomChanged && !p.spec.LintMissingDependencies {
		return &PluginResult{}, nil
	}

	bom, err := p.resolveBOM(entity, sbomBytes)
	if err != nil {
		return nil, err
	}
	if bomChanged {
		log.Printf("Processed SBOM %s for entity %s: %d components", bom.Name, entity.GetQName(), len(bom.Components))
	}

	now := time.Now()
	observations := make(map[string]catalog.Observation)
	var removedObservations []string

	if bomChanged {
		observations[JFrogXrayPluginTarget] = catalog.Observation{
			Value:     api.MustMarshalJSON(bom),
			UpdatedAt: now,
			Producer:  "JFrogXrayPlugin",
			Version:   version,
		}
	}

	if p.spec.LintMissingDependencies {
		// Lint runs every tick, even when the BOM is cached, because the
		// entity's declared dependencies may have changed in the YAML repo
		// and we have no cheap way to detect that.
		idx := p.getCatalogIndex(args.Repository)
		missing, _ := p.detectDependencyMismatches(bom, entity, idx, args.Repository)
		if len(missing) > 0 {
			finding := api.LintFinding{
				CreateTime: now,
				Message:    fmt.Sprintf("Dependencies found in BOM, but missing in entity: %s", strings.Join(missing, ",")),
			}
			observations[JFrogXrayPluginLintTarget] = catalog.Observation{
				Value:     api.MustMarshalJSON(finding),
				UpdatedAt: now,
				Producer:  "JFrogXrayPlugin",
				Version:   version,
			}
		} else {
			// Clear any previously-emitted finding so it doesn't linger after
			// the entity's declared dependencies catch up with the BOM.
			removedObservations = append(removedObservations, JFrogXrayPluginLintTarget)
		}
	}

	return &PluginResult{
		Observations:        observations,
		RemovedObservations: removedObservations,
	}, nil
}

// resolveBOM returns the parsed/filtered BOM either from freshly downloaded
// sbomBytes (when non-nil) or from the entity's previously stored observation.
func (p *JFrogXrayPlugin) resolveBOM(entity catalog.Entity, sbomBytes []byte) (*sbom.MiniBOM, error) {
	if sbomBytes != nil {
		sbomObj, err := sbom.Parse(sbomBytes)
		if err != nil {
			return nil, fmt.Errorf("cannot parse SBOM: %w", err)
		}
		bom, err := sbom.FilterComponents(sbomObj, p.spec.ComponentsFilter)
		if err != nil {
			return nil, fmt.Errorf("filtering components: %w", err)
		}
		return bom, nil
	}

	status := entity.GetStatus()
	if status == nil {
		return nil, fmt.Errorf("BOM not refreshed and no stored observation for %v", entity.GetQName())
	}
	prev, ok := status.Observations[JFrogXrayPluginTarget]
	if !ok {
		return nil, fmt.Errorf("BOM not refreshed and no stored observation for %v", entity.GetQName())
	}
	var bom sbom.MiniBOM
	if err := json.Unmarshal(prev.Value, &bom); err != nil {
		return nil, fmt.Errorf("failed to parse stored BOM for %v: %w", entity.GetQName(), err)
	}
	return &bom, nil
}

// detectDependencyMismatches compares bom.Components and all dependencies (including API usage)
// of the given entity.
// It returns mismatches as two lists: missing for components present in bom but missing
// in entity's deps; extra for deps present in entity deps, but missing in bom.
// It only works for Component entities.
func (p *JFrogXrayPlugin) detectDependencyMismatches(bom *sbom.MiniBOM, entity catalog.Entity, fullIdx *catalogIndex, repository *repo.Repository) (missing []string, extra []string) {
	comp, ok := entity.(*catalog.Component)
	if !ok {
		return nil, nil
	}

	// 1. Resolve all declared dependencies of this component.
	declared := make(map[string]catalog.Entity)
	add := func(refs []*catalog.LabelRef) {
		for _, r := range refs {
			if e := repository.Entity(r.Ref); e != nil {
				declared[e.GetRef().String()] = e
			}
		}
	}
	add(comp.Spec.DependsOn)
	add(comp.Spec.ConsumesAPIs)
	add(comp.Spec.ProvidesAPIs)

	// 2. Create index for declared dependencies for fast matching.
	declaredEntities := make([]catalog.Entity, 0, len(declared))
	for _, e := range declared {
		declaredEntities = append(declaredEntities, e)
	}
	declaredIdx := newCatalogIndexFromEntities(declaredEntities)

	// 3. Compare SBOM components with declared dependencies.
	matchedRefs := make(map[string]bool)

	for _, raw := range bom.Components {
		bc := parseGAV(raw)
		if e := declaredIdx.matchEntity(bc); e != nil {
			matchedRefs[e.GetRef().String()] = true
		} else {
			// Not in declared deps. Is it in the catalog at all?
			if e := fullIdx.matchEntity(bc); e != nil {
				// Yes, it's a catalog entity.
				// Add to missing if it's different from 'entity' itself.
				// (Entities annotated as lint-ignored are filtered out of fullIdx
				// in newCatalogIndexFromEntities, so they cannot match here.)
				if !e.GetRef().Equal(entity.GetRef()) {
					missing = append(missing, raw)
				}
			}
		}
	}

	// 4. Any declared dependency missing from the SBOM?
	for ref, e := range declared {
		if !matchedRefs[ref] {
			extra = append(extra, e.GetQName())
		}
	}

	slices.Sort(missing)
	slices.Sort(extra)
	return
}

type gav struct {
	g, a, v string
}

func parseGAV(s string) gav {
	parts := strings.Split(s, ":")
	if len(parts) == 1 {
		return gav{a: parts[0]}
	}
	if len(parts) == 2 {
		return gav{g: parts[0], a: parts[1]}
	}
	return gav{g: parts[0], a: parts[1], v: parts[2]}
}

type indexedEntity struct {
	entity catalog.Entity
	coords gav
}

type catalogIndex struct {
	entities []indexedEntity
}

func newCatalogIndexFromEntities(allEntities []catalog.Entity) *catalogIndex {
	entities := make([]indexedEntity, 0, len(allEntities))
	for _, e := range allEntities {
		annotations := e.GetMetadata().Annotations
		if annotations[JFrogXrayPluginLintIgnoreAnnotation] == "true" {
			continue
		}
		info := indexedEntity{entity: e, coords: gav{a: e.GetMetadata().Name}}
		if coords, ok := annotations[JFrogXrayPluginCoordsAnnotation]; ok {
			info.coords = parseGAV(coords)
		}
		entities = append(entities, info)
	}

	slices.SortFunc(entities, func(i, j indexedEntity) int {
		if cmp := strings.Compare(i.coords.a, j.coords.a); cmp != 0 {
			return cmp
		}
		return strings.Compare(i.coords.g, j.coords.g)
	})

	return &catalogIndex{
		entities: entities,
	}
}

func (idx *catalogIndex) matchEntity(bc gav) catalog.Entity {
	if bc.a == "" {
		return nil
	}

	// sort.Search returns the smallest index i such that f(i) is true.
	start := sort.Search(len(idx.entities), func(i int) bool {
		return idx.entities[i].coords.a >= bc.a
	})

	// Scan only the block with matching artifactId.
	for j := start; j < len(idx.entities) && idx.entities[j].coords.a == bc.a; j++ {
		info := idx.entities[j]
		if info.coords.g == "" || bc.g == "" || info.coords.g == bc.g {
			return info.entity
		}
	}
	return nil
}
