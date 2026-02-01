package web

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/yuin/goldmark"
)

var (
	errFunctionUndefined = errors.New("template function not defined for request")
)

func undefinedTemplateFunction(s any) (string, error) {
	return "", errFunctionUndefined
}

func isCloneable(e catalog.Entity) bool {
	k := e.GetKind()
	return k == catalog.KindAPI || k == catalog.KindComponent || k == catalog.KindResource
}

func anyToRef(s any) (*catalog.Ref, error) {
	switch r := s.(type) {
	case string:
		e, err := catalog.ParseRef(r)
		if err != nil {
			return nil, fmt.Errorf("invalid entity reference string for entityURL: %v", err)
		}
		return e, nil
	case *catalog.Ref:
		return r, nil
	case *catalog.LabelRef:
		return r.Ref, nil
	case catalog.Entity:
		return r.GetRef(), nil
	}
	return nil, fmt.Errorf("anyToRef: invalid argument type %T", s)
}

func toEntityURLWithContext(ctx context.Context, s any) (string, error) {
	entityRef, err := anyToRef(s)
	if err != nil {
		return "", err
	}
	if ref := ctx.Value(ctxRef); ref != nil {
		return fmt.Sprintf("/ui/ref/%s/-/entities/%s", ref, entityRef.String()), nil
	}

	return "/ui/entities/" + url.PathEscape(entityRef.String()), nil
}

// kindPath returns the <kind> URL path part for the given entity kind
func kindPath(kind catalog.Kind) string {
	switch kind {
	case catalog.KindComponent:
		return "components"
	case catalog.KindResource:
		return "resources"
	case catalog.KindSystem:
		return "systems"
	case catalog.KindGroup:
		return "groups"
	case catalog.KindDomain:
		return "domains"
	case catalog.KindAPI:
		return "apis"
	}
	panic(fmt.Sprintf("Unhandled kind: %s", kind))
}

func uiURLWithContext(ctx context.Context, suffix string) string {
	suffix = strings.TrimPrefix(suffix, "/")
	if ref := ctx.Value(ctxRef); ref != nil {
		return fmt.Sprintf("/ui/ref/%s/-/%s", ref, suffix)
	}
	return "/ui/" + suffix
}

func toListURLWithContext(ctx context.Context, kind catalog.Kind) string {
	return uiURLWithContext(ctx, kindPath(kind))
}

func toURLWithContext(ctx context.Context, s any) (string, error) {
	entityRef, err := anyToRef(s)
	if err != nil {
		return "", err
	}

	if entityRef.Kind == "" {
		return "", fmt.Errorf("entity reference has no kind: set: %v", entityRef)
	}
	var pathPrefix string
	if ref := ctx.Value(ctxRef); ref != nil {
		pathPrefix = fmt.Sprintf("/ui/ref/%s/-/%s", ref, kindPath(entityRef.Kind))
	} else {
		pathPrefix = fmt.Sprintf("/ui/%s", kindPath(entityRef.Kind))
	}
	return pathPrefix + "/" + url.PathEscape(entityRef.QName()), nil
}

func toGraphURLWithContext(ctx context.Context, entityRefs []string) string {
	base := uiURLWithContext(ctx, "graph")
	vals := url.Values{}
	for _, e := range entityRefs {
		vals.Add("e", e)
	}
	return base + "?" + vals.Encode()
}

// refOption holds data for rendering <option>s for different git refs.
type refOption struct {
	Ref      string // The git reference name
	URL      string // The URL to navigate to (typically an /absolute/path Request URI)
	Selected bool   // Whether the option should be marked as selected.
}

// refOptions generates navigation links to switch the current view to a different reference.
// It extracts the current page context ("tail") from the raw RequestURI to bypass any middleware
// modifications, transforming paths like "/ui/catalog" or "/ui/ref/main/-/catalog" into
// the target format "/ui/ref/<new-ref>/-/<tail>".
func refOptions(refs []string, currentRef string, r *http.Request) []refOption {
	// 1. Snapshot the raw request to bypass middleware mutations.
	// Split off the query immediately.
	rawPath, rawQuery, _ := strings.Cut(r.RequestURI, "?")

	// 2. Isolate the "tail" (the logical page path, e.g., "components/<id>").
	// We check for the delimiter "/-/" to see if we are already in a ref view.
	var tail string
	if _, after, found := strings.Cut(rawPath, "/-/"); found {
		tail = after
	} else {
		// We are in a top-level view. Safely remove the root anchor.
		tail = strings.TrimPrefix(rawPath, "/ui")
		tail = strings.TrimPrefix(tail, "/")
	}

	// 3. Pre-format the query string for reuse
	queryString := ""
	if rawQuery != "" {
		queryString = "?" + rawQuery
	}

	// 4. Construct the new URLs
	result := make([]refOption, 0, len(refs))
	for _, ref := range refs {
		result = append(result, refOption{
			Ref: ref,
			// Construct: /ui/ref/<ref>/-/<tail>?<query>
			URL:      fmt.Sprintf("/ui/ref/%s/-/%s%s", ref, tail, queryString),
			Selected: ref == currentRef,
		})
	}

	return result
}

// entitySummary returns e's title and description concatenated.
// The text is truncated at 157 characters and an ellipse (...)
// is appended if the text's total length exceeds 160 characters.
func entitySummary(e catalog.Entity) string {
	if e == nil {
		return ""
	}
	m := e.GetMetadata()
	if m == nil {
		return ""
	}
	var elems []string
	if m.Title != "" {
		elems = append(elems, m.Title)
	}
	if m.Description != "" {
		elems = append(elems, m.Description)
	}
	summary := strings.Join(elems, " • ")

	rs := []rune(summary)
	if len(rs) > 160 {
		return string(rs[:157]) + "..."
	}
	return summary
}

func parentSystem(e catalog.Entity) *catalog.Ref {
	if sp, ok := e.(catalog.SystemPart); ok {
		sys := sp.GetSystem()
		if sys != nil && !sys.Equal(e.GetRef()) {
			return sys
		}
	}
	return nil
}

type FormattedChip struct {
	DisplayKey string // short form of the label key, e.g. "foo" for "example.com/foo".
	Key        string // original label key, empty for tags
	Value      string // original label value or the tag value.
	Type       string // "label" or "tag"
}

func (c *FormattedChip) DisplayString() string {
	if c.DisplayKey == "" {
		return c.Value
	}
	return c.DisplayKey + ": " + c.Value
}

func compareFormattedChip(a, b FormattedChip) int {
	if c := cmp.Compare(a.Type, b.Type); c != 0 {
		return c
	}
	if c := cmp.Compare(a.DisplayKey, b.DisplayKey); c != 0 {
		return c
	}
	return cmp.Compare(a.Value, b.Value)
}

// formatTags returns the given tags in sorted order.
func formatTags(tags []string) []FormattedChip {
	result := make([]FormattedChip, len(tags))
	for i, tag := range tags {
		result[i] = FormattedChip{
			Value: tag,
			Type:  "tag",
		}
	}
	slices.SortFunc(result, compareFormattedChip)
	return result
}

// formatLabels formats all labels of the given metadata.
// Labels that have "qualified" keys, such as "example.com/some-label"
// will be formatted in their simple form ("some-label") for readability,
// *unless* the simple label is ambiguous. In that case, the qualified
// key is used.
func formatLabels(meta *catalog.Metadata) []FormattedChip {
	if meta == nil || len(meta.Labels) == 0 {
		return nil
	}
	unqualify := func(s string) string {
		if _, tail, ok := strings.Cut(s, "/"); ok {
			return tail
		}
		return s
	}
	counts := make(map[string]int, len(meta.Labels))
	for k := range meta.Labels {
		counts[unqualify(k)]++
	}

	result := make([]FormattedChip, 0, len(meta.Labels))
	for k, v := range meta.Labels {
		s := unqualify(k)
		show := s
		if counts[s] > 1 {
			show = k // keep fully-qualified to disambiguate
		}
		result = append(result, FormattedChip{
			DisplayKey: show,
			Key:        k,
			Value:      v,
			Type:       "label",
		})
	}
	slices.SortFunc(result, compareFormattedChip)

	return result
}

// SVG Icons

var (
	svgIcons = map[string]string{
		// A loupe icon
		"Search": `<svg width="24px" height="24px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M14.9536 14.9458L21 21M17 10C17 13.866 13.866 17 10 17C6.13401 17 3 13.866 3 10C3 6.13401 6.13401 3 10 3C13.866 3 17 6.13401 17 10Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`,
		// A graph of three connected nodes.
		"Graph": `<svg width="24px" height="24px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M11.109 14.546C11 14.7599 11 15.0399 11 15.6V17.4C11 17.9601 11 18.2401 11.109 18.454C11.2049 18.6422 11.3578 18.7951 11.546 18.891C11.7599 19 12.0399 19 12.6 19H14.4C14.9601 19 15.2401 19 15.454 18.891C15.6422 18.7951 15.7951 18.6422 15.891 18.454C16 18.2401 16 17.9601 16 17.4V15.6C16 15.0399 16 14.7599 15.891 14.546C15.7951 14.3578 15.6422 14.2049 15.454 14.109C15.2401 14 14.9601 14 14.4 14H12.6C12.0399 14 11.7599 14 11.546 14.109C11.3578 14.2049 11.2049 14.3578 11.109 14.546ZM11.109 14.546L7.7386 9.67415M8 7.5H16M4.6 10H6.4C6.96005 10 7.24008 10 7.45399 9.89101C7.64215 9.79513 7.79513 9.64215 7.89101 9.45399C8 9.24008 8 8.96005 8 8.4V6.6C8 6.03995 8 5.75992 7.89101 5.54601C7.79513 5.35785 7.64215 5.20487 7.45399 5.10899C7.24008 5 6.96005 5 6.4 5H4.6C4.03995 5 3.75992 5 3.54601 5.10899C3.35785 5.20487 3.20487 5.35785 3.10899 5.54601C3 5.75992 3 6.03995 3 6.6V8.4C3 8.96005 3 9.24008 3.10899 9.45399C3.20487 9.64215 3.35785 9.79513 3.54601 9.89101C3.75992 10 4.03995 10 4.6 10ZM17.6 10H19.4C19.9601 10 20.2401 10 20.454 9.89101C20.6422 9.79513 20.7951 9.64215 20.891 9.45399C21 9.24008 21 8.96005 21 8.4V6.6C21 6.03995 21 5.75992 20.891 5.54601C20.7951 5.35785 20.6422 5.20487 20.454 5.10899C20.2401 5 19.9601 5 19.4 5H17.6C17.0399 5 16.7599 5 16.546 5.10899C16.3578 5.20487 16.2049 5.35785 16.109 5.54601C16 5.75992 16 6.03995 16 6.6V8.4C16 8.96005 16 9.24008 16.109 9.45399C16.2049 9.64215 16.3578 9.79513 16.546 9.89101C16.7599 10 17.0399 10 17.6 10Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`,
	}
)

//
// Navigation bar utilities
//

type NavBar struct {
	Items []*NavBarItem
}

type NavBarItem struct {
	Path   string
	Title  string
	Icon   string
	Active bool
}

func NavItem(path, title string) *NavBarItem {
	return &NavBarItem{
		Path:  path,
		Title: title,
	}
}

func NavIcon(path, iconKey string) *NavBarItem {
	return &NavBarItem{
		Path: path,
		Icon: svgIcons[iconKey],
	}
}

func (i *NavBarItem) Display() template.HTML {
	if i.Icon != "" {
		return template.HTML(i.Icon)
	}
	return template.HTML(i.Title)
}

func NewNavBar(items ...*NavBarItem) *NavBar {
	return &NavBar{Items: items}
}

// SetActive sets the .Active flags of all NavItems of this NavBar.
// The item whose path is a prefix (or equal to) requestURI is set to active.
func (ns *NavBar) SetActive(requestURI string) *NavBar {
	path, _, _ := strings.Cut(requestURI, "?") // Cut off query params
	// Clean path to handle double slashes etc.
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	path = strings.TrimSuffix(path, "/")

	for _, n := range ns.Items {
		// Also clean nav path
		navPath := strings.TrimSuffix(n.Path, "/")
		if path == navPath || strings.HasPrefix(path, navPath+"/") {
			n.Active = true
		} else {
			n.Active = false
		}
	}
	return ns
}

func markdown(input string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(input), &buf); err != nil {
		return "", fmt.Errorf("failed to process markdown: %v", err)
	}
	return template.HTML(buf.String()), nil
}

func setQueryParam(r *http.Request, key, value string) *url.URL {
	u, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		// Cannot parse RequestURI. This is a failure mode. Return request URL unchanged.
		return r.URL
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u
}

type ccAttr struct {
	Name  string
	Value string
}

type ccRow struct {
	Columns []string
}

type ccTable struct {
	Headers []string
	Rows    []*ccRow
}

// CustomContent represents content that is displayed in the detail view
// and is specified in an entity annotation.
type CustomContent struct {
	Heading string
	Text    string   // Text to be presented as-is.
	Items   []string // Items to be rendered as an <ul> list.
	Attrs   []ccAttr // Items to be rendered as a key-value <table>.
	Table   *ccTable // Items to be rendered in a custom <table>.
	Code    string   // Preformatted code (typically JSON)
	Rank    int      // Used to order multiple custom content items.
}

func customContentFromAnnotations(meta *catalog.Metadata, configMap map[string]*config.AnnotationBasedContent) ([]*CustomContent, error) {
	if len(meta.Annotations) == 0 || len(configMap) == 0 {
		return nil, nil
	}
	var result []*CustomContent
	for k, abc := range configMap {
		anno, ok := meta.Annotations[k]
		if !ok {
			continue
		}
		cc, err := newCustomContent(abc, anno)
		if err != nil {
			return nil, fmt.Errorf("invalid custom content: %v", err)
		}
		cc.Rank = abc.Rank
		result = append(result, cc)
	}
	slices.SortFunc(result, func(a, b *CustomContent) int {
		if c := cmp.Compare(a.Rank, b.Rank); c != 0 {
			return c
		}
		return cmp.Compare(a.Heading, b.Heading)
	})

	return result, nil
}

func newCustomContent(abc *config.AnnotationBasedContent, annotationValue string) (*CustomContent, error) {
	cc := &CustomContent{
		Heading: abc.Heading,
	}
	switch abc.Style {
	case "text", "":
		cc.Text = annotationValue
	case "list":
		var items []string
		if err := json.Unmarshal([]byte(annotationValue), &items); err != nil {
			return nil, fmt.Errorf("not a valid list of strings: %v", err)
		}
		cc.Items = items
	case "attrs":
		var dict map[string]any
		if err := json.Unmarshal([]byte(annotationValue), &dict); err != nil {
			return nil, fmt.Errorf("not a valid JSON object: %v", err)
		}
		keys := make([]string, 0, len(dict))
		for k := range dict {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			cc.Attrs = append(cc.Attrs, ccAttr{
				Name:  k,
				Value: fmt.Sprintf("%v", dict[k]),
			})
		}
	case "table":
		var items []map[string]any
		if err := json.Unmarshal([]byte(annotationValue), &items); err != nil {
			return nil, fmt.Errorf("not a valid list of objects: %v", err)
		}
		t := &ccTable{
			Headers: make([]string, len(abc.Columns)),
			Rows:    make([]*ccRow, len(items)),
		}
		hasHeaders := false
		for i, c := range abc.Columns {
			t.Headers[i] = c.Header
			if c.Header != "" {
				hasHeaders = true
			}
		}
		if !hasHeaders {
			t.Headers = nil
		}
		for i, item := range items {
			r := &ccRow{
				Columns: make([]string, len(abc.Columns)),
			}
			for j, c := range abc.Columns {
				var buf bytes.Buffer
				if err := c.DataTemplate().Execute(&buf, item); err != nil {
					r.Columns[j] = fmt.Sprintf("template error: %v", err)
				} else {
					r.Columns[j] = buf.String()
				}
			}
			t.Rows[i] = r
		}
		cc.Table = t
	case "json":
		var raw any
		if err := json.Unmarshal([]byte(annotationValue), &raw); err != nil {
			return nil, fmt.Errorf("not a valid JSON object: %v", err)
		}
		indentedJSON, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to indent JSON: %v", err)
		}
		cc.Code = string(indentedJSON)
	default:
		return nil, fmt.Errorf("invalid custom content style (must be text|list|attrs|table|json): %s", abc.Style)
	}
	return cc, nil
}
