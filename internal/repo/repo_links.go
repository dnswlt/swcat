package repo

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
	starlarkinterp "github.com/dnswlt/swcat/internal/starlark"
)

// linkTemplateFuncs defines custom template functions available in link templates.
var linkTemplateFuncs = template.FuncMap{
	// first returns the first non-empty string from the given arguments.
	"first": func(args ...string) string {
		for _, arg := range args {
			if arg != "" {
				return arg
			}
		}
		return ""
	},
	// pathEscape percent-encodes a string for use in URL path segments.
	"pathEscape": url.PathEscape,
	// queryParams builds url.Values from an even-numbered list of key-value pairs.
	"queryParams": func(kvs ...string) (url.Values, error) {
		if len(kvs)%2 != 0 {
			return nil, fmt.Errorf("queryParams: requires even number of arguments, got %d", len(kvs))
		}
		v := url.Values{}
		for i := 0; i < len(kvs); i += 2 {
			v.Add(kvs[i], kvs[i+1])
		}
		return v, nil
	},
	// addQueryParams appends query parameters to a base URL, merging with any existing ones.
	"addQueryParams": func(base string, params url.Values) (string, error) {
		u, err := url.Parse(base)
		if err != nil {
			return "", fmt.Errorf("addQueryParams: invalid base URL: %w", err)
		}
		q := u.Query()
		for k, vs := range params {
			for _, val := range vs {
				q.Add(k, val)
			}
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	},
	"toGVK": func(kind string) string {
		mapping := map[string]string{
			"Deployment":       "apps~v1~Deployment",
			"StatefulSet":      "apps~v1~StatefulSet",
			"DaemonSet":        "apps~v1~DaemonSet",
			"ReplicaSet":       "apps~v1~ReplicaSet",
			"CronJob":          "batch~v1~CronJob",
			"Job":              "batch~v1~Job",
			"DeploymentConfig": "apps.openshift.io~v1~DeploymentConfig",
		}

		// Return exact GVK match if found
		if gvk, exists := mapping[kind]; exists {
			return gvk
		}

		// Fallback for core resources like Pod, Service, ConfigMap
		return "core~v1~" + kind
	},
}

type nameval struct {
	Name  string
	Value string
}

// linkTemplateContext holds the values and provides the methods that
// are available in the context of URL and title generating templates.
type linkTemplateContext struct {
	// repo and entity are used by this type's methods to retrieve data.
	repo   *Repository
	entity catalog.Entity

	// Fields containing specific contextual data relevant to the link generation.
	Annotation nameval

	// Deprecated template values retained for historical catalog revisions.
	Version   *catalog.Version
	MultiLink MultiLinkEntry
}

func (c *linkTemplateContext) Metadata() *catalog.Metadata {
	return c.entity.GetMetadata()
}

func (c *linkTemplateContext) System() *catalog.System {
	switch x := c.entity.(type) {
	case *catalog.System:
		return x
	case catalog.SystemPart:
		return c.repo.System(x.GetSystem())
	default:
		return nil
	}
}

func (c *linkTemplateContext) Domain() *catalog.Domain {
	if d, ok := c.entity.(*catalog.Domain); ok {
		return d
	}
	sys := c.System()
	if sys == nil {
		return nil
	}
	return c.repo.Domain(sys.Spec.Domain)
}

func (c *linkTemplateContext) GetAnnotation(key string) string {
	return c.entity.GetMetadata().Annotations[key]
}

func (c *linkTemplateContext) IAnnotation(key string) string {
	if v, ok := c.repo.IAnnotation(c.entity, key); ok {
		return v
	}
	return ""
}

func (c *linkTemplateContext) Label(key string) string {
	return c.entity.GetMetadata().Labels[key]
}

func (c *linkTemplateContext) ILabel(key string) string {
	if v, ok := c.repo.ILabel(c.entity, key); ok {
		return v
	}
	return ""
}

// linkGenerator holds compiled templates for an annotation-based link rule.
type linkGenerator struct {
	url        *template.Template
	title      *template.Template
	icon       string
	typ        string
	annotation string

	// Deprecated expansion settings retained for historical catalog revisions.
	legacyHasVersion    bool
	legacyMultiLinks    []MultiLinkEntry
	legacyMultiLinkData string
}

// legacyAutomaticLinkGenerator adds the filter used by the deprecated
// automaticLinks configuration. Template rendering remains shared with
// annotation-based links, which also retain legacy expansion support.
type legacyAutomaticLinkGenerator struct {
	linkGenerator
	eval *query.Evaluator
}

type starlarkLinkGenerator struct {
	eval    *query.Evaluator
	program *starlarkinterp.Program
}

// isValidAbsoluteURL checks if a string is a valid, absolute URL
// with a scheme (like "http" or "https") and a host.
func isValidAbsoluteURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

// renderURL executes the URL template and validates the result.
func (g *linkGenerator) renderURL(data linkTemplateContext) (string, error) {
	var sb strings.Builder
	// Need to pass &data here so the pointer receiver methods are available.
	if err := g.url.Execute(&sb, &data); err != nil {
		return "", fmt.Errorf("failed to execute URL template for %v: %v", data.entity.GetRef(), err)
	}
	u := sb.String()
	if !isValidAbsoluteURL(u) {
		return "", fmt.Errorf("invalid URL for %v: %q", data.entity.GetRef(), u)
	}
	return u, nil
}

// renderTitle executes the title template.
func (g *linkGenerator) renderTitle(data linkTemplateContext) (string, error) {
	var sb strings.Builder
	// Need to pass &data here so the pointer receiver methods are available.
	if err := g.title.Execute(&sb, &data); err != nil {
		return "", fmt.Errorf("failed to execute title template in entity %v: %v", data.entity.GetRef(), err)
	}
	return sb.String(), nil
}

// generateLinks produces links for an annotation-based template. New
// configurations take the single-link path; historical configurations may
// still request the deprecated expansion behavior.
func (g *linkGenerator) generateLinks(r *Repository, e catalog.Entity) ([]*catalog.Link, error) {
	baseCtx := linkTemplateContext{repo: r, entity: e}
	if g.annotation != "" {
		if annotValue, ok := e.GetMetadata().Annotations[g.annotation]; ok {
			baseCtx.Annotation = nameval{g.annotation, annotValue}
		}
	}

	if g.usesLegacyExpansion() {
		return g.generateLegacyExpandedLinks(baseCtx, e)
	}
	return g.generateSingleLink(baseCtx)
}

func (g *linkGenerator) generateSingleLink(ctx linkTemplateContext) ([]*catalog.Link, error) {
	u, err := g.renderURL(ctx)
	if err != nil {
		return nil, err
	}
	title, err := g.renderTitle(ctx)
	if err != nil {
		return nil, err
	}
	return []*catalog.Link{{
		Title:       title,
		URL:         u,
		Icon:        g.icon,
		Type:        g.typ,
		IsGenerated: true,
	}}, nil
}

func (g *linkGenerator) usesLegacyExpansion() bool {
	return g.legacyHasVersion || len(g.legacyMultiLinks) > 0 || g.legacyMultiLinkData != ""
}

// generateLegacyExpandedLinks retains multi-link, inherited multi-link data,
// and implicit per-version expansion for historical swcat.yml revisions.
func (g *linkGenerator) generateLegacyExpandedLinks(baseCtx linkTemplateContext, e catalog.Entity) ([]*catalog.Link, error) {
	multiLinks, err := g.resolveLegacyMultiLinks(baseCtx, e)
	if err != nil {
		return nil, err
	}
	if len(multiLinks) > 0 {
		return g.generateLegacyMultiLinks(baseCtx, e, multiLinks)
	}
	if g.legacyHasVersion {
		if ap, ok := e.(*catalog.API); ok && len(ap.Spec.Versions) > 0 {
			return g.generateLegacyVersionLinks(baseCtx, ap)
		}
	}
	return g.generateSingleLink(baseCtx)
}

func (g *linkGenerator) resolveLegacyMultiLinks(baseCtx linkTemplateContext, e catalog.Entity) ([]MultiLinkEntry, error) {
	multiLinks := g.legacyMultiLinks
	if g.legacyMultiLinkData != "" {
		v := baseCtx.IAnnotation("swcat/data-" + g.legacyMultiLinkData)
		if v != "" {
			var entries []MultiLinkEntry
			if err := json.Unmarshal([]byte(v), &entries); err != nil {
				return nil, fmt.Errorf("invalid JSON in annotation swcat/data-%s for %v: %v",
					g.legacyMultiLinkData, e.GetRef(), err)
			}
			multiLinks = entries
		}
	}
	return multiLinks, nil
}

func (g *linkGenerator) generateLegacyMultiLinks(baseCtx linkTemplateContext, e catalog.Entity, multiLinks []MultiLinkEntry) ([]*catalog.Link, error) {
	// For non-versioned entities we use one nil entry to run the loop once.
	var versions []*catalog.Version
	if g.legacyHasVersion {
		if ap, ok := e.(*catalog.API); ok {
			for i := range ap.Spec.Versions {
				versions = append(versions, &ap.Spec.Versions[i].Version)
			}
		}
	}
	if len(versions) == 0 {
		versions = []*catalog.Version{nil}
	}

	links := make([]*catalog.Link, 0, len(versions)*len(multiLinks))
	for _, ver := range versions {
		// The group title is version-specific but independent of the multi-link.
		verCtx := baseCtx
		if ver != nil {
			verCtx.Version = ver
		}
		groupTitle, err := g.renderTitle(verCtx)
		if err != nil {
			return nil, err
		}
		for _, ml := range multiLinks {
			mlCtx := verCtx
			mlCtx.MultiLink = ml
			u, err := g.renderURL(mlCtx)
			if err != nil {
				return nil, err
			}
			links = append(links, &catalog.Link{
				Title:       groupTitle + " (" + ml.Label + ")",
				URL:         u,
				Icon:        g.icon,
				Type:        g.typ,
				IsGenerated: true,
				GroupInfo:   &catalog.LinkGroupInfo{Group: groupTitle, Label: ml.Label},
			})
		}
	}
	return links, nil
}

func (g *linkGenerator) generateLegacyVersionLinks(baseCtx linkTemplateContext, api *catalog.API) ([]*catalog.Link, error) {
	links := make([]*catalog.Link, 0, len(api.Spec.Versions))
	for _, ver := range api.Spec.Versions {
		verCtx := baseCtx
		verCtx.Version = &ver.Version
		u, err := g.renderURL(verCtx)
		if err != nil {
			return nil, err
		}
		title, err := g.renderTitle(verCtx)
		if err != nil {
			return nil, err
		}
		links = append(links, &catalog.Link{
			Title:       title,
			URL:         u,
			Icon:        g.icon,
			Type:        g.typ,
			IsGenerated: true,
		})
	}
	return links, nil
}

// compileLinkTemplates compiles a URL+title template pair, applying missingkey=error.
func compileLinkTemplates(urlStr, titleStr, errContext string) (*template.Template, *template.Template, error) {
	urlTmpl, err := template.New("url").Funcs(linkTemplateFuncs).Parse(urlStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URL template for %s: %v", errContext, err)
	}
	urlTmpl.Option("missingkey=error")
	titleTmpl, err := template.New("title").Funcs(linkTemplateFuncs).Parse(titleStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid title template for %s: %v", errContext, err)
	}
	titleTmpl.Option("missingkey=error")
	return urlTmpl, titleTmpl, nil
}

// prepareAnnotationLinkTemplates compiles the supported one-to-one annotation
// rules and any deprecated expansion settings found in historical configs.
func (r *Repository) prepareAnnotationLinkTemplates() ([]linkGenerator, error) {
	var generators []linkGenerator
	versionPlaceholderRE := regexp.MustCompile(`\{\{\s*\.Version\b`)

	for annot, abl := range r.config.AnnotationBasedLinks {
		if abl == nil {
			return nil, fmt.Errorf("annotation-based label for %q is nil", annot)
		}
		if strings.TrimSpace(abl.URL) == "" {
			return nil, fmt.Errorf("annotation-based label for %q has an empty URL", annot)
		}
		urlTmpl, titleTmpl, err := compileLinkTemplates(abl.URL, abl.Title, fmt.Sprintf("annotation %q", annot))
		if err != nil {
			return nil, err
		}
		generators = append(generators, linkGenerator{
			url:                 urlTmpl,
			title:               titleTmpl,
			icon:                abl.Icon,
			typ:                 abl.Type,
			annotation:          annot,
			legacyHasVersion:    versionPlaceholderRE.MatchString(abl.URL + " " + abl.Title),
			legacyMultiLinks:    abl.MultiLinks,
			legacyMultiLinkData: abl.MultiLinkData,
		})
	}
	return generators, nil
}

// prepareLegacyAutomaticLinkTemplates retains automaticLinks support for
// historical Git refs. New configurations use Starlark links.
func (r *Repository) prepareLegacyAutomaticLinkTemplates() ([]legacyAutomaticLinkGenerator, error) {
	var generators []legacyAutomaticLinkGenerator
	for _, al := range r.config.AutomaticLinks {
		if al == nil {
			continue
		}
		if strings.TrimSpace(al.Filter) == "" {
			return nil, fmt.Errorf("automatic link has an empty filter")
		}
		if strings.TrimSpace(al.URL) == "" {
			return nil, fmt.Errorf("automatic link has an empty URL")
		}
		expr, err := query.Parse(al.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter expression %q: %v", al.Filter, err)
		}
		urlTmpl, titleTmpl, err := compileLinkTemplates(al.URL, al.Title, "automatic link")
		if err != nil {
			return nil, err
		}
		generators = append(generators, legacyAutomaticLinkGenerator{
			linkGenerator: linkGenerator{
				url:                 urlTmpl,
				title:               titleTmpl,
				icon:                al.Icon,
				typ:                 al.Type,
				legacyMultiLinks:    al.MultiLinks,
				legacyMultiLinkData: al.MultiLinkData,
			},
			eval: query.NewEvaluator(expr),
		})
	}

	return generators, nil
}

func (r *Repository) prepareStarlarkLinks() ([]starlarkLinkGenerator, error) {
	generators := make([]starlarkLinkGenerator, 0, len(r.config.StarlarkLinks))
	for i, link := range r.config.StarlarkLinks {
		if link == nil {
			return nil, fmt.Errorf("starlarkLinks[%d] is null", i)
		}
		if strings.TrimSpace(link.Filter) == "" {
			return nil, fmt.Errorf("starlarkLinks[%d] has an empty filter", i)
		}
		if link.program == nil {
			return nil, fmt.Errorf("starlarkLinks[%d] file %q was not loaded", i, link.File)
		}
		expr, err := query.Parse(link.Filter)
		if err != nil {
			return nil, fmt.Errorf("starlarkLinks[%d] has invalid filter expression %q: %w", i, link.Filter, err)
		}
		generators = append(generators, starlarkLinkGenerator{
			eval:    query.NewEvaluator(expr),
			program: link.program,
		})
	}
	return generators, nil
}

// generateLegacyAutomaticLinks evaluates deprecated automaticLinks rules for
// catalogs loaded from historical Git refs.
func (r *Repository) generateLegacyAutomaticLinks(e catalog.Entity, generators []legacyAutomaticLinkGenerator) ([]*catalog.Link, error) {
	var links []*catalog.Link
	for i := range generators {
		g := &generators[i]
		matches, err := g.eval.Matches(e)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate filter for entity %v: %v", e.GetRef(), err)
		}
		if !matches {
			continue
		}
		generated, err := g.generateLinks(r, e)
		if err != nil {
			return nil, err
		}
		links = append(links, generated...)
	}
	return links, nil
}

func (r *Repository) addGeneratedLinks() error {
	annotationTemplates, err := r.prepareAnnotationLinkTemplates()
	if err != nil {
		return err
	}
	legacyAutomaticTemplates, err := r.prepareLegacyAutomaticLinkTemplates()
	if err != nil {
		return err
	}
	starlarkGenerators, err := r.prepareStarlarkLinks()
	if err != nil {
		return err
	}

	for _, e := range r.allEntities {
		meta := e.GetMetadata()
		// Check that no generated links already exist (that would be a programming error)
		if slices.ContainsFunc(meta.Links, func(l *catalog.Link) bool {
			return l.IsGenerated
		}) {
			panic(fmt.Sprintf("addGeneratedLinks called on entity %s that already has generated links", e.GetRef()))
		}
		var links []*catalog.Link
		for i := range annotationTemplates {
			g := &annotationTemplates[i]
			value, ok := meta.Annotations[g.annotation]
			if !ok || value == "" {
				continue
			}
			newLinks, err := g.generateLinks(r, e)
			if err != nil {
				return err
			}
			links = append(links, newLinks...)
		}
		legacyLinks, err := r.generateLegacyAutomaticLinks(e, legacyAutomaticTemplates)
		if err != nil {
			return err
		}
		links = append(links, legacyLinks...)
		for _, generator := range starlarkGenerators {
			matches, err := generator.eval.Matches(e)
			if err != nil {
				return fmt.Errorf("failed to evaluate Starlark link filter for entity %v: %w", e.GetRef(), err)
			}
			if !matches {
				continue
			}
			generated, err := generator.program.Links(e, r)
			if err != nil {
				return fmt.Errorf("failed to execute Starlark link file %q for entity %v: %w", generator.program.Filename(), e.GetRef(), err)
			}
			for _, link := range generated {
				if !isValidAbsoluteURL(link.URL) {
					return fmt.Errorf("invalid URL from Starlark link file %q for %v: %q", generator.program.Filename(), e.GetRef(), link.URL)
				}
				catalogLink := &catalog.Link{
					URL:         link.URL,
					Title:       link.Title,
					Icon:        link.Icon,
					Type:        link.Type,
					IsGenerated: true,
				}
				if link.Group != "" {
					catalogLink.GroupInfo = &catalog.LinkGroupInfo{
						Group: link.Group,
						Label: link.Label,
					}
				}
				links = append(links, catalogLink)
			}
		}

		meta.Links = append(meta.Links, links...)
		slices.SortFunc(meta.Links, func(a, b *catalog.Link) int {
			if c := cmp.Compare(a.Title, b.Title); c != 0 {
				return c
			}
			return cmp.Compare(a.URL, b.URL)
		})
	}
	return nil
}
