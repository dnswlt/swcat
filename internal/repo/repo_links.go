package repo

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
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
	Version    *catalog.Version
	MultiLink  MultiLinkEntry
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
	e := c.entity
	for e != nil {
		if v, ok := e.GetMetadata().Annotations[key]; ok {
			return v
		}
		r := e.GetParent()
		if r == nil {
			break
		}
		e = c.repo.Entity(r)
	}
	return ""
}

func (c *linkTemplateContext) Label(key string) string {
	return c.entity.GetMetadata().Labels[key]
}

func (c *linkTemplateContext) ILabel(key string) string {
	e := c.entity
	for e != nil {
		if v, ok := e.GetMetadata().Labels[key]; ok {
			return v
		}
		r := e.GetParent()
		if r == nil {
			break
		}
		e = c.repo.Entity(r)
	}
	return ""
}

// linkGenerator holds compiled templates and metadata for one auto-generated link rule.
// Exactly one of annotation or eval is set, determining when the rule fires.
type linkGenerator struct {
	url           *template.Template
	title         *template.Template
	icon          string
	typ           string
	hasVersion    bool             // if true, generates per-version links for versioned APIs
	multiLinks    []MultiLinkEntry // if non-empty, generates per-entry links
	multiLinkData string           // if non-empty, generates per-entry links from the swcat/data-{multiLinkData} annotation.
	annotation    string           // annotation-based: fire when entity has this annotation
	eval          *query.Evaluator // automatic: fire when entity matches this filter
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

// generateLinks produces all links for entity e.
func (g *linkGenerator) generateLinks(r *Repository, e catalog.Entity) ([]*catalog.Link, error) {
	baseCtx := linkTemplateContext{repo: r, entity: e}
	if g.annotation != "" {
		if annotValue, ok := e.GetMetadata().Annotations[g.annotation]; ok {
			baseCtx.Annotation = nameval{g.annotation, annotValue}
		}
	}

	multiLinks := g.multiLinks
	if g.multiLinkData != "" {
		// Find multi-links via annotation reference.
		v := baseCtx.IAnnotation("swcat/data-" + g.multiLinkData)
		if v != "" {
			var entries []MultiLinkEntry
			if err := json.Unmarshal([]byte(v), &entries); err != nil {
				return nil, fmt.Errorf("invalid JSON in annotation swcat/data-%s for %v: %v",
					g.multiLinkData, e.GetRef(), err)
			}
			multiLinks = entries
		}
	}

	if len(multiLinks) > 0 {
		// Collect versions to iterate over. For non-versioned entities (or when
		// hasVersion is false) we use a single nil entry to run the loop once.
		var versions []*catalog.Version
		if g.hasVersion {
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
			// Build per-version data (without MultiLink) to render the group title.
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

	if g.hasVersion {
		if ap, ok := e.(*catalog.API); ok && len(ap.Spec.Versions) > 0 {
			links := make([]*catalog.Link, 0, len(ap.Spec.Versions))
			for _, ver := range ap.Spec.Versions {
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
	}

	// Single link (no multi-links, no multiple versions).
	u, err := g.renderURL(baseCtx)
	if err != nil {
		return nil, err
	}
	title, err := g.renderTitle(baseCtx)
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

// prepareLinkTemplates compiles all url and title templates found in the config.
func (r *Repository) prepareLinkTemplates() ([]linkGenerator, error) {
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
			url:           urlTmpl,
			title:         titleTmpl,
			icon:          abl.Icon,
			typ:           abl.Type,
			hasVersion:    versionPlaceholderRE.MatchString(abl.URL + " " + abl.Title),
			multiLinks:    abl.MultiLinks,
			multiLinkData: abl.MultiLinkData,
			annotation:    annot,
		})
	}

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
		generators = append(generators, linkGenerator{
			url:           urlTmpl,
			title:         titleTmpl,
			icon:          al.Icon,
			typ:           al.Type,
			multiLinks:    al.MultiLinks,
			multiLinkData: al.MultiLinkData,
			eval:          query.NewEvaluator(expr),
		})
	}

	return generators, nil
}

func (r *Repository) addGeneratedLinks() error {
	tmpls, err := r.prepareLinkTemplates()
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
		for i := range tmpls {
			g := &tmpls[i]
			if g.annotation != "" {
				value, ok := meta.Annotations[g.annotation]
				if !ok || value == "" {
					continue
				}
			} else {
				matches, err := g.eval.Matches(e)
				if err != nil {
					return fmt.Errorf("failed to evaluate filter for entity %v: %v", e.GetRef(), err)
				}
				if !matches {
					continue
				}
			}
			newLinks, err := g.generateLinks(r, e)
			if err != nil {
				return err
			}
			links = append(links, newLinks...)
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
