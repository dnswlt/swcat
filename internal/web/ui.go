package web

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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

func toEntitiesListURL(ctx context.Context) string {
	if ref := ctx.Value(ctxRef); ref != nil {
		return fmt.Sprintf("/ui/ref/%s/-/entities", ref)
	}
	return "/ui/entities"
}

func toListURLWithContext(ctx context.Context, kind catalog.Kind) string {
	kp := kindPath(kind)
	if ref := ctx.Value(ctxRef); ref != nil {
		return fmt.Sprintf("/ui/ref/%s/-/%s", ref, kp)
	}
	return fmt.Sprintf("/ui/%s", kp)
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

//
// Navigation bar utilities
//

type NavBar []*NavBarItem

type NavBarItem struct {
	path        string
	queryParams map[string]string
	params      []string
	Title       string
	Active      bool
}

func (n *NavBarItem) URI() string {
	var u url.URL
	u.Path = n.path
	q := make(url.Values)
	for k, v := range n.queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (n *NavBarItem) Params(params ...string) *NavBarItem {
	n.params = params
	return n
}

func (n *NavBarItem) ParamsList() string {
	return strings.Join(n.params, ",")
}

type NavQueryParam struct {
	Key   string
	Value string
}

func NavItem(path, title string) *NavBarItem {
	return &NavBarItem{
		path:        path,
		Title:       title,
		queryParams: make(map[string]string),
	}
}

func NewNavBar(items ...*NavBarItem) NavBar {
	return items
}

// SetActive sets the .Active flags of all NavItems of this NavBar.
// The item whose path is a prefix (or equal to) activePath is set to active.
func (ns NavBar) SetActive(activePath string) NavBar {
	segments := strings.Split(activePath, "/")
	for _, n := range ns {
		navSegments := strings.Split(n.path, "/")
		if len(navSegments) > len(segments) {
			continue
		}
		isPrefix := true
		for i, segment := range navSegments {
			if segments[i] != segment {
				isPrefix = false
				break
			}
		}
		n.Active = isPrefix
	}
	return ns
}

func (ns NavBar) SetParam(key, value string) NavBar {
	for _, n := range ns {
		if slices.Contains(n.params, key) {
			n.queryParams[key] = value
		}
	}
	return ns
}

func (ns NavBar) SetParams(q url.Values) NavBar {
	for k := range q {
		if v := q.Get(k); v != "" {
			ns = ns.SetParam(k, q.Get(k))
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

func setQueryParam(u *url.URL, key, value string) *url.URL {
	u2 := *u
	q := u2.Query()
	q.Set(key, value)
	u2.RawQuery = q.Encode()
	return &u2
}

// CustomContent represents content that is displayed in the detail view
// and is specified in an entity annotation.
type CustomContent struct {
	Heading string
	Text    string   // Text to be presented as-is.
	Items   []string // Items to be rendered as an <ul> list.
	Code    string   // Preformatted code (typically JSON)
}

func customContentFromAnnotations(meta *catalog.Metadata, configMap map[string]config.AnnotationBasedContent) ([]*CustomContent, error) {
	if len(meta.Annotations) == 0 || len(configMap) == 0 {
		return nil, nil
	}
	var result []*CustomContent
	for k, abc := range configMap {
		anno, ok := meta.Annotations[k]
		if !ok {
			continue
		}
		cc, err := newCustomContent(abc.Heading, anno, abc.Style)
		if err != nil {
			return nil, fmt.Errorf("invalid custom content: %v", err)
		}
		result = append(result, cc)
	}
	return result, nil
}

func newCustomContent(heading, content, style string) (*CustomContent, error) {
	cc := &CustomContent{
		Heading: heading,
	}
	switch style {
	case "text", "":
		cc.Text = content
	case "list":
		var items []string
		if err := json.Unmarshal([]byte(content), &items); err != nil {
			return nil, fmt.Errorf("not a valid list of strings: %v", err)
		}
		cc.Items = items
	case "json":
		var raw any
		if err := json.Unmarshal([]byte(content), &raw); err != nil {
			return nil, fmt.Errorf("not a valid JSON object: %v", err)
		}
		indentedJSON, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to indent JSON: %v", err)
		}
		cc.Code = string(indentedJSON)
	default:
		return nil, fmt.Errorf("invalid custom content style (must be text|list|json): %s", style)
	}
	return cc, nil
}
