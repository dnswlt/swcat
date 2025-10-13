package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/yuin/goldmark"
)

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

func toEntityURL(s any) (string, error) {
	entityRef, err := anyToRef(s)
	if err != nil {
		return "", err
	}
	return "/ui/entities/" + url.PathEscape(entityRef.String()), nil
}

func toURL(s any) (string, error) {
	entityRef, err := anyToRef(s)
	if err != nil {
		return "", err
	}

	if entityRef.Kind == "" {
		return "", fmt.Errorf("entity reference has no kind: set: %v", entityRef)
	}
	var path string
	switch entityRef.Kind {
	case "component":
		path = "/ui/components/"
	case "resource":
		path = "/ui/resources/"
	case "system":
		path = "/ui/systems/"
	case "group":
		path = "/ui/groups/"
	case "domain":
		path = "/ui/domains/"
	case "api":
		path = "/ui/apis/"
	default:
		return "", fmt.Errorf("unsupported kind %q in entityURL", entityRef.Kind)
	}
	return path + url.PathEscape(entityRef.QName()), nil
}

// formatTags returns the given tags in sorted order.
func formatTags(tags []string) []string {
	out := make([]string, len(tags))
	copy(out, tags)
	slices.Sort(out)
	return out
}

// formatLabels formats all labels of the given metadata.
// Labels that have "qualified" keys, such as "example.com/some-label"
// will be formatted in their simple form ("some-label") for readability,
// *unless* the simple label is ambiguous. In that case, the qualified
// key is used.
func formatLabels(meta *catalog.Metadata) []string {
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

	type pair struct {
		showKey string
		val     string
	}
	pairs := make([]pair, 0, len(meta.Labels))
	for k, v := range meta.Labels {
		s := unqualify(k)
		show := s
		if counts[s] > 1 {
			show = k // keep fully-qualified to disambiguate
		}
		pairs = append(pairs, pair{showKey: show, val: v})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].showKey != pairs[j].showKey {
			return pairs[i].showKey < pairs[j].showKey
		}
		return pairs[i].val < pairs[j].val
	})

	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = fmt.Sprintf("%s: %s", p.showKey, p.val)
	}
	return out

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
