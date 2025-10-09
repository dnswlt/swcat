package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
)

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

func (ns NavBar) SetActive(activePath string) NavBar {
	activePath = strings.TrimSuffix(activePath, "/")
	for _, n := range ns {
		if activePath == strings.TrimSuffix(n.path, "/") {
			n.Active = true
			break
		}
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

func Markdown(input string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(input), &buf); err != nil {
		return "", fmt.Errorf("failed to process markdown: %v", err)
	}
	return template.HTML(buf.String()), nil
}
