package starlark

import (
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

type fakeCatalog struct {
	entities    map[string]catalog.Entity
	annotations map[string]string
}

func (c *fakeCatalog) Entity(ref *catalog.Ref) catalog.Entity {
	return c.entities[ref.String()]
}

func (c *fakeCatalog) IAnnotation(_ catalog.Entity, key string) (string, bool) {
	value, ok := c.annotations[key]
	return value, ok
}

func testEntities() (*catalog.Component, *fakeCatalog) {
	systemRef := &catalog.Ref{Kind: catalog.KindSystem, Namespace: "default", Name: "payments"}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "payments", Namespace: "default", Title: "Payments"},
		Spec:     &catalog.SystemSpec{Type: "service"},
	}
	component := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name:      "checkout",
			Namespace: "default",
			Labels:    map[string]string{"language": "go"},
		},
		Spec: &catalog.ComponentSpec{
			Type:   "service",
			System: systemRef,
		},
	}
	return component, &fakeCatalog{
		entities: map[string]catalog.Entity{
			system.GetRef().String(): system,
		},
		annotations: map[string]string{
			"example.com/environments": `[{"name":"prod","host":"prod.example.com"}]`,
		},
	}
}

func TestProgramLinks(t *testing.T) {
	component, catalog_ := testEntities()
	program, err := Compile("links.star", []byte(`
def links(entity):
    system = lookup_ref(entity["componentSpec"]["system"])
    environments = json.decode(iannotation("example.com/environments"))
    environment = environments[0]
    return [link(
        url="https://{host}/{component}".format(
            host=environment["host"],
            component=entity["metadata"]["name"],
        ),
        title="Deploy {system}".format(system=system["metadata"]["title"]),
        icon="cloud",
        type="deployment",
        group="Deployments",
        label=environment["name"],
    )]
`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	links, err := program.Links(component, catalog_)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	want := []Link{{
		URL:   "https://prod.example.com/checkout",
		Title: "Deploy Payments",
		Icon:  "cloud",
		Type:  "deployment",
		Group: "Deployments",
		Label: "prod",
	}}
	if len(links) != len(want) || links[0] != want[0] {
		t.Fatalf("Links = %#v, want %#v", links, want)
	}
}

func TestCompileRejectsInvalidPrograms(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{name: "syntax error", source: `def links(:`, wantErr: "got ':'"},
		{name: "undefined name", source: `def links(entity): return unknown(entity)`, wantErr: "undefined: unknown"},
		{name: "load", source: "load(\"helpers.star\", \"helper\")\ndef links(entity): return []", wantErr: "load statements are not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile("broken.star", []byte(tt.source))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Compile error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestProgramRejectsInvalidResultsAndRuntimeErrors(t *testing.T) {
	component, catalog_ := testEntities()
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name:    "missing function",
			source:  `answer = 42`,
			wantErr: "must define links(entity)",
		},
		{
			name:    "non-callable links",
			source:  `links = 42`,
			wantErr: "links is int, want function",
		},
		{
			name:    "non-list result",
			source:  `def links(entity): return link(url="https://example.com")`,
			wantErr: "returned link, want list",
		},
		{
			name:    "dictionary result item",
			source:  `def links(entity): return [{"url": "https://example.com"}]`,
			wantErr: "returned dict at index 0, want link",
		},
		{
			name:    "constructor type error",
			source:  `def links(entity): return [link(url=123)]`,
			wantErr: "for parameter \"url\": got int, want string",
		},
		{
			name:    "incomplete group",
			source:  `def links(entity): return [link(url="https://example.com", group="Examples")]`,
			wantErr: "group and label must either both be set",
		},
		{
			name:    "top-level runtime error",
			source:  "broken = 1 // 0\ndef links(entity): return []",
			wantErr: "floored division by zero",
		},
		{
			name:    "runtime error",
			source:  `def links(entity): return [1 // 0]`,
			wantErr: "floored division by zero",
		},
		{
			name:    "immutable entity",
			source:  `def links(entity): entity["kind"] = "Domain"; return []`,
			wantErr: "cannot insert into frozen hash table",
		},
		{
			name: "execution limit",
			source: `
def links(entity):
    total = 0
    for i in range(2000000):
        total += i
    return []
`,
			wantErr: "too many steps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, err := Compile("invalid.star", []byte(tt.source))
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			_, err = program.Links(component, catalog_)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Links error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLookupRefReturnsNoneForMissingEntity(t *testing.T) {
	component, catalog_ := testEntities()
	program, err := Compile("missing-ref.star", []byte(`
def links(entity):
    missing = {"kind": "System", "namespace": "default", "name": "missing"}
    if lookup_ref(missing) == None:
        return []
    return [link(url="https://unexpected.example.com")]
`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	links, err := program.Links(component, catalog_)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("Links = %#v, want none", links)
	}
}

func TestLookupRefOnlyExposesAuthoredLinks(t *testing.T) {
	component, catalog_ := testEntities()
	system := catalog_.entities[component.Spec.System.String()]
	system.GetMetadata().Links = []*catalog.Link{
		{URL: "https://docs.example.com", Title: "Docs"},
		{URL: "https://generated.example.com", Title: "Generated", IsGenerated: true},
	}
	program, err := Compile("authored-links.star", []byte(`
def links(entity):
    system = lookup_ref(entity["componentSpec"]["system"])
    count = len(system["metadata"].get("links", []))
    return [link(url="https://example.com/{count}".format(count=count))]
`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	links, err := program.Links(component, catalog_)
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if len(links) != 1 || links[0].URL != "https://example.com/1" {
		t.Fatalf("Links = %#v, want one authored related-entity link", links)
	}
}
