package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
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
	// Annotation in which to find a JSON list of "[groupId:]artifactId"
	// strings of dependencies that should never be declared as missing during linting.
	// This is the usual "lint:ignore" hook that's always needed for weird edge
	// cases, to avoid flooding the system with lint warnings that no none looks at.
	JFrogXrayPluginLintIgnoreAnnotation = "swcat-plugins/jfrog-xray-ignore"
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

// fetchSBOM retrieves the SBOM for the latest available semver version of
// image in repository, trying up to the three most recent versions.
func (p *JFrogXrayPlugin) fetchSBOM(ctx context.Context, repository, image string) ([]byte, error) {
	tags, err := p.client.ListDockerTags(ctx, repository, image)
	if err != nil {
		return nil, err
	}
	versions := latestSemverVersions(tags, 3)
	if len(versions) == 0 {
		return nil, fmt.Errorf("no valid semver tags found for %s/%s", repository, image)
	}
	var lastErr error
	for _, version := range versions {
		data, err := p.client.XrayExportDetails(ctx, repository, image, version)
		if err != nil {
			log.Printf("fetchSBOM: skipping %s:%s: %v", image, version, err)
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("could not download SBOM for %s/%s (tried %v): %w", repository, image, versions, lastErr)
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
	sbomStr, err := p.fetchSBOM(ctx, repository, image)
	if err != nil {
		return nil, err
	}

	// Process SBOM and extract flat string list of component coordinates
	// (group:artifact).
	sbomObj, err := sbom.Parse(sbomStr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse SBOM: %w", err)
	}
	bom, err := sbom.FilterComponents(sbomObj, p.spec.ComponentsFilter)
	if err != nil {
		return nil, fmt.Errorf("filtering components: %w", err)
	}
	log.Printf("Processed SBOM %s for entity %s: %d components", bom.Name, entity.GetQName(), len(bom.Components))

	if args.Repository == nil {
		return nil, fmt.Errorf("repository not set in plugin args")
	}
	idx := p.newCatalogIndexFromEntities(args.Repository.AllEntities())

	now := time.Now()

	observations := map[string]catalog.Observation{
		JFrogXrayPluginTarget: {
			Value:     api.MustMarshalJSON(bom),
			UpdatedAt: now,
			Producer:  "JFrogXrayPlugin",
			Version:   bom.Version,
		},
	}

	if p.spec.LintMissingDependencies {
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
				Version:   bom.Version,
			}
		}
	}

	return &PluginResult{
		Observations: observations,
	}, nil
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

	ignored := make(map[string]bool)
	if val, ok := entity.GetMetadata().Annotations[JFrogXrayPluginLintIgnoreAnnotation]; ok {
		var list []string
		err := json.Unmarshal([]byte(val), &list)
		if err == nil {
			for _, s := range list {
				ignored[s] = true
			}
		}
	}

	isIgnored := func(bc gav) bool {
		if len(ignored) == 0 {
			return false
		}
		if ignored[bc.a] { // artifactId
			return true
		}
		if bc.g != "" && ignored[bc.g+":"+bc.a] { // groupId:artifactId
			return true
		}
		return false
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
	declaredIdx := p.newCatalogIndexFromEntities(declaredEntities)

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
				// Add to mising if it's different from 'entity' itself and not explicitly ignored.
				if !isIgnored(bc) && !e.GetRef().Equal(entity.GetRef()) {
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

func (p *JFrogXrayPlugin) newCatalogIndexFromEntities(allEntities []catalog.Entity) *catalogIndex {
	entities := make([]indexedEntity, 0, len(allEntities))
	for _, e := range allEntities {
		info := indexedEntity{entity: e, coords: gav{a: e.GetMetadata().Name}}
		if coords, ok := e.GetMetadata().Annotations[JFrogXrayPluginCoordsAnnotation]; ok {
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
