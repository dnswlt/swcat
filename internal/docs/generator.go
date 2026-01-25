package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
)

// Generator builds the documentation structure.
type Generator struct {
	repo *repo.Repository
}

func NewGenerator(r *repo.Repository) *Generator {
	return &Generator{repo: r}
}

// shouldGenerate returns true unless the entity has the "swcat/gen-docs" annotation set to "false".
func (g *Generator) shouldGenerate(e catalog.Entity) bool {
	if e == nil || e.GetMetadata() == nil {
		return false
	}
	val, ok := e.GetMetadata().Annotations[catalog.AnnotGenDocs]
	if ok && strings.ToLower(val) == "false" {
		return false
	}
	return true
}

// Generate builds the documentation in the output directory.
func (g *Generator) Generate(outputDir string) error {
	repo := g.repo

	// 1. Generate Main Index (Domains)
	allDomains := repo.FindDomains("")
	var domains []*catalog.Domain
	for _, d := range allDomains {
		if g.shouldGenerate(d) {
			domains = append(domains, d)
		}
	}
	sort.Slice(domains, func(i, j int) bool {
		return domains[i].GetQName() < domains[j].GetQName()
	})

	if err := g.generateRootIndex(outputDir, domains); err != nil {
		return err
	}

	// 2. Generate Domain Indexes (Systems)
	for _, domain := range domains {
		domainDir := filepath.Join(outputDir, domain.GetRef().Name)
		systems := domain.GetSystems()
		var domainSystems []*catalog.System
		for _, s := range systems {
			system := repo.System(s)
			if g.shouldGenerate(system) {
				domainSystems = append(domainSystems, system)
			}
		}
		sort.Slice(domainSystems, func(i, j int) bool {
			return domainSystems[i].GetQName() < domainSystems[j].GetQName()
		})
		if err := g.generateDomainIndex(domainDir, domain, domainSystems); err != nil {
			return err
		}

		// 3. Generate System Indexes (Entities)
		for _, system := range domainSystems {
			systemDir := filepath.Join(domainDir, system.GetRef().Name)

			// Find all entities belonging to this system
			var components []*catalog.Component
			var apis []*catalog.API
			var resources []*catalog.Resource

			// Components
			componentRefs := system.GetComponents()
			for _, c := range componentRefs {
				component := repo.Component(c)
				if g.shouldGenerate(component) {
					components = append(components, component)
				}
			}
			// APIs
			apiRefs := system.GetAPIs()
			for _, a := range apiRefs {
				api := repo.API(a)
				if g.shouldGenerate(api) {
					apis = append(apis, api)
				}
			}
			// Resources
			resourceRefs := system.GetResources()
			for _, r := range resourceRefs {
				resource := repo.Resource(r)
				if g.shouldGenerate(resource) {
					resources = append(resources, resource)
				}
			}

			// Sort by Name
			sort.Slice(components, func(i, j int) bool {
				return components[i].GetQName() < components[j].GetQName()
			})
			sort.Slice(apis, func(i, j int) bool {
				return apis[i].GetQName() < apis[j].GetQName()
			})
			sort.Slice(resources, func(i, j int) bool {
				return resources[i].GetQName() < resources[j].GetQName()
			})

			if err := g.generateSystemIndex(systemDir, system, components, apis, resources); err != nil {
				return err
			}

			// 4. Generate Entity Placeholders

			// Components
			for _, c := range components {
				entityDir := filepath.Join(systemDir, "components")
				if err := os.MkdirAll(entityDir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", entityDir, err)
				}
				filename := filepath.Join(entityDir, c.GetRef().Name+".md")
				if err := g.ensureComponentDoc(filename, c); err != nil {
					return err
				}
			}

			// APIs
			for _, a := range apis {
				entityDir := filepath.Join(systemDir, "apis")
				if err := os.MkdirAll(entityDir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", entityDir, err)
				}
				filename := filepath.Join(entityDir, a.GetRef().Name+".md")
				if err := g.ensureAPIDoc(filename, a); err != nil {
					return err
				}
			}

			// Resources
			for _, r := range resources {
				entityDir := filepath.Join(systemDir, "resources")
				if err := os.MkdirAll(entityDir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", entityDir, err)
				}
				filename := filepath.Join(entityDir, r.GetRef().Name+".md")
				if err := g.ensureResourceDoc(filename, r); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (g *Generator) generateRootIndex(dir string, domains []*catalog.Domain) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	f, err := os.Create(filepath.Join(dir, "index.md"))
	if err != nil {
		return fmt.Errorf("failed to create index.md in %s: %w", dir, err)
	}
	defer f.Close()

	data := struct {
		Title string
		Items []*catalog.Domain
	}{
		Title: "Home",
		Items: domains,
	}

	if err := domainsTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute template domains: %w", err)
	}
	return nil
}

func (g *Generator) generateDomainIndex(dir string, domain *catalog.Domain, systems []*catalog.System) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	f, err := os.Create(filepath.Join(dir, "index.md"))
	if err != nil {
		return fmt.Errorf("failed to create index.md in %s: %w", dir, err)
	}
	defer f.Close()

	data := struct {
		Title string
		Items []*catalog.System
	}{
		Title: domain.GetRef().Name,
		Items: systems,
	}

	if err := systemsTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute template systems: %w", err)
	}
	return nil
}

func (g *Generator) generateSystemIndex(dir string, system *catalog.System, components []*catalog.Component, apis []*catalog.API, resources []*catalog.Resource) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	f, err := os.Create(filepath.Join(dir, "index.md"))
	if err != nil {
		return fmt.Errorf("failed to create index.md in %s: %w", dir, err)
	}
	defer f.Close()

	data := struct {
		Title      string
		Components []*catalog.Component
		APIs       []*catalog.API
		Resources  []*catalog.Resource
	}{
		Title:      system.GetRef().Name,
		Components: components,
		APIs:       apis,
		Resources:  resources,
	}

	if err := systemEntitiesTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute system entities template: %w", err)
	}
	return nil
}

func (g *Generator) ensureComponentDoc(filename string, c *catalog.Component) error {
	if _, err := os.Stat(filename); err == nil {
		return nil
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create component doc %s: %w", filename, err)
	}
	defer f.Close()

	return componentTemplate.Execute(f, c)
}

func (g *Generator) ensureAPIDoc(filename string, a *catalog.API) error {
	if _, err := os.Stat(filename); err == nil {
		return nil
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create api doc %s: %w", filename, err)
	}
	defer f.Close()

	return apiTemplate.Execute(f, a)
}

func (g *Generator) ensureResourceDoc(filename string, r *catalog.Resource) error {
	if _, err := os.Stat(filename); err == nil {
		return nil
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create resource doc %s: %w", filename, err)
	}
	defer f.Close()

	return resourceTemplate.Execute(f, r)
}

// Templates

var domainsTemplate = template.Must(template.New("domains").Parse(`---
title: {{ .Title }}
---
<!-- Auto-generated by swcat gen-docs. DO NOT EDIT. -->
# {{ .Title }}

## Domains

{{ range .Items -}}
* [{{ .GetRef.Name }}]({{ .GetRef.Name }}/index.md){{ if .GetMetadata.Title }} - *{{ .GetMetadata.Title }}*{{ end }}
{{ end }}
`))

var systemsTemplate = template.Must(template.New("systems").Parse(`---
title: {{ .Title }}
---
<!-- Auto-generated by swcat gen-docs. DO NOT EDIT. -->
# {{ .Title }}

## Systems

{{ range .Items -}}
* [{{ .GetRef.Name }}]({{ .GetRef.Name }}/index.md){{ if .GetMetadata.Title }} - *{{ .GetMetadata.Title }}*{{ end }}
{{ end }}
`))

var systemEntitiesTemplate = template.Must(template.New("systemEntities").Parse(`---
title: {{ .Title }}
---
<!-- Auto-generated by swcat gen-docs. DO NOT EDIT. -->
# {{ .Title }}

{{ if .Components -}}
## Components

{{ range .Components -}}
* [{{ .GetRef.Name }}](components/{{ .GetRef.Name }}.md){{ if .GetMetadata.Title }} - *{{ .GetMetadata.Title }}*{{ end }}
{{ end }}
{{- end }}

{{ if .APIs -}}
## APIs

{{ range .APIs -}}
* [{{ .GetRef.Name }}](apis/{{ .GetRef.Name }}.md){{ if .GetMetadata.Title }} - *{{ .GetMetadata.Title }}*{{ end }}
{{ end }}
{{- end }}

{{ if .Resources -}}
## Resources

{{ range .Resources -}}
* [{{ .GetRef.Name }}](resources/{{ .GetRef.Name }}.md){{ if .GetMetadata.Title }} - *{{ .GetMetadata.Title }}*{{ end }}
{{ end }}
{{- end }}
`))

var componentTemplate = template.Must(template.New("component").Parse(`# {{ .GetRef.Name }}

**Kind**: {{ .GetKind }}

{{ .GetMetadata.Description }}

## Details

> !!! warning
>     This is an auto-generated placeholder for {{ .GetRef.Name }}.
> 
>     TODO: Add details here.
`))

var apiTemplate = template.Must(template.New("api").Parse(`# {{ .GetRef.Name }}

**Kind**: {{ .GetKind }}

{{ .GetMetadata.Description }}

## Details

> !!! warning
>     This is an auto-generated placeholder for {{ .GetRef.Name }}.
> 
>     TODO: Add details here.
`))

var resourceTemplate = template.Must(template.New("resource").Parse(`# {{ .GetRef.Name }}

**Kind**: {{ .GetKind }}

{{ .GetMetadata.Description }}

## Details

> !!! warning
>     This is an auto-generated placeholder for {{ .GetRef.Name }}.
> 
>     TODO: Add details here.
`))
