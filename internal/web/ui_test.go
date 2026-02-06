package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/google/go-cmp/cmp"
)

func TestFormatLabels(t *testing.T) {
	type tc struct {
		name      string
		meta      *catalog.Metadata
		want      []string
		wantIsNil bool
	}

	cases := []tc{
		{
			name:      "nil meta",
			meta:      nil,
			want:      nil,
			wantIsNil: true,
		},
		{
			name: "no labels",
			meta: &catalog.Metadata{
				Labels: map[string]string{},
			},
			want:      nil,
			wantIsNil: true,
		},
		{
			name: "single unqualified label",
			meta: &catalog.Metadata{
				Labels: map[string]string{"env": "prod"},
			},
			want: []string{"env: prod"},
		},
		{
			name: "qualified unique tails -> show simple keys",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/role": "api",
					"acme.io/tier":     "gold",
				},
			},
			want: []string{
				"role: api",
				"tier: gold",
			},
		},
		{
			name: "ambiguous tails -> keep qualified keys",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/role": "api",
					"acme.io/role":     "db",
				},
			},
			want: []string{
				"acme.io/role: db",
				"example.com/role: api",
			},
		},
		{
			name: "mix of unique and ambiguous",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/role": "api",
					"acme.io/role":     "db",
					"region":           "eu-west",
				},
			},
			want: []string{
				"acme.io/role: db",
				"example.com/role: api",
				"region: eu-west",
			},
		},
		{
			name: "multiple slashes in key -> cut at first slash",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"a/b/c": "x",
					"d/e/f": "y",
				},
			},
			want: []string{
				"b/c: x",
				"e/f: y",
			},
		},
		{
			name: "empty values are allowed",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"team": "",
				},
			},
			want: []string{
				"team: ",
			},
		},
		{
			name: "stable order by displayed key then value",
			meta: &catalog.Metadata{
				Labels: map[string]string{
					"example.com/a": "2",
					"acme.io/a":     "1",
					"b":             "zz",
				},
			},
			want: []string{
				"acme.io/a: 1",
				"b: zz",
				"example.com/a: 2",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatLabels(c.meta)

			if c.wantIsNil {
				if got != nil {
					t.Fatalf("expected nil slice, got: %v", got)
				}
				return
			}

			gotDisplay := make([]string, len(got))
			for i, chip := range got {
				gotDisplay[i] = chip.DisplayString()
			}
			if diff := cmp.Diff(c.want, gotDisplay); diff != "" {
				t.Fatalf("formatLabels() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetQueryParam(t *testing.T) {
	tests := []struct {
		name       string
		requestURI string
		key        string
		value      string
		wantURL    string
	}{
		{
			name:       "add new param",
			requestURI: "/ui/components",
			key:        "q",
			value:      "test",
			wantURL:    "/ui/components?q=test",
		},
		{
			name:       "modify existing param",
			requestURI: "/ui/components?q=old&foo=bar",
			key:        "q",
			value:      "new",
			wantURL:    "/ui/components?foo=bar&q=new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.requestURI, nil)
			gotURL := setQueryParam(req, tt.key, tt.value)

			if gotURL.RequestURI() != tt.wantURL {
				t.Errorf("setQueryParam(%q, %q, %q) got %q, want %q", tt.requestURI, tt.key, tt.value, gotURL.RequestURI(), tt.wantURL)
			}
		})
	}
}

func TestRefOptions(t *testing.T) {
	type args struct {
		refs       []string
		currentRef string
		requestURI string
	}
	tests := []struct {
		name string
		args args
		want []refOption
	}{
		{
			name: "simple path, no existing ref, check selected and URLs",
			args: args{
				refs:       []string{"main", "dev"},
				currentRef: "main",
				requestURI: "/ui/components",
			},
			want: []refOption{
				{Ref: "main", URL: "/ui/ref/main/-/components", Selected: true},
				{Ref: "dev", URL: "/ui/ref/dev/-/components", Selected: false},
			},
		},
		{
			name: "existing ref in path, switch ref, preserve query",
			args: args{
				refs:       []string{"main", "feature-x"},
				currentRef: "feature-x",
				requestURI: "/ui/ref/main/-/systems?env=prod",
			},
			want: []refOption{
				{Ref: "main", URL: "/ui/ref/main/-/systems?env=prod", Selected: false},
				{Ref: "feature-x", URL: "/ui/ref/feature-x/-/systems?env=prod", Selected: true},
			},
		},
		{
			name: "root path with query",
			args: args{
				refs:       []string{"main"},
				currentRef: "main",
				requestURI: "/ui?search=all",
			},
			want: []refOption{
				{Ref: "main", URL: "/ui/ref/main/-/?search=all", Selected: true},
			},
		},
		{
			name: "empty refs list",
			args: args{
				refs:       []string{},
				currentRef: "main",
				requestURI: "/ui/components",
			},
			want: []refOption{},
		},
		{
			name: "ref with multiple path segments",
			args: args{
				refs:       []string{"bugfix/b199", "main"},
				currentRef: "bugfix/b199",
				requestURI: "/ui/ref/main/-/components/my-component",
			},
			want: []refOption{
				{Ref: "bugfix/b199", URL: "/ui/ref/bugfix/b199/-/components/my-component", Selected: true},
				{Ref: "main", URL: "/ui/ref/main/-/components/my-component", Selected: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.args.requestURI, nil)

			got := refOptions(tt.args.refs, tt.args.currentRef, req)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("refOptions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewCustomContent(t *testing.T) {
	tests := []struct {
		name    string
		heading string
		content string
		style   string
		want    *CustomContent
		wantErr bool
	}{
		{
			name:    "text style",
			heading: "My Text",
			content: "Hello World",
			style:   "text",
			want: &CustomContent{
				Heading: "My Text",
				Text:    "Hello World",
			},
		},
		{
			name:    "list style valid",
			heading: "My List",
			content: `["a", "b"]`,
			style:   "list",
			want: &CustomContent{
				Heading: "My List",
				Items:   []string{"a", "b"},
			},
		},
		{
			name:    "json style valid",
			heading: "My JSON",
			content: `{"key": "value"}`,
			style:   "json",
			want: &CustomContent{
				Heading: "My JSON",
				Code:    "{\n  \"key\": \"value\"\n}",
			},
		},
		{
			name:    "attrs style valid",
			heading: "My Attrs",
			content: `{"b": 2, "a": "1"}`,
			style:   "attrs",
			want: &CustomContent{
				Heading: "My Attrs",
				Attrs: []ccAttr{
					{Name: "a", Value: "1"},
					{Name: "b", Value: "2"},
				},
			},
		},
		{
			name:    "attrs style invalid json",
			heading: "My Attrs",
			content: `invalid`,
			style:   "attrs",
			wantErr: true,
		},
		{
			name:    "unknown style",
			heading: "Unknown",
			content: "foo",
			style:   "unknown",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abc := &config.AnnotationBasedContent{
				Heading: tt.heading,
				Style:   tt.style,
			}
			got, err := newCustomContent(abc, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("newCustomContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("newCustomContent() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestApplyMagicLinks(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text, no refs",
			input: "this is some text",
			want:  "this is some text",
		},
		{
			name:  "single component ref",
			input: "check out component:cache-loader",
			want:  "check out [component:cache-loader](/ui/components/cache-loader)",
		},
		{
			name:  "api ref with namespace",
			input: "use api:external/some-api",
			want:  "use [api:external/some-api](/ui/apis/external%2Fsome-api)",
		},
		{
			name:  "multiple refs",
			input: "system:sys1 depends on resource:res1",
			want:  "[system:sys1](/ui/systems/sys1) depends on [resource:res1](/ui/resources/res1)",
		},
		{
			name:  "ref with context",
			input: "component:c1",
			want:  "[component:c1](/ui/components/c1)",
		},
		{
			name:  "invalid ref kind",
			input: "unknown:foo",
			want:  "unknown:foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyMagicLinks(ctx, tt.input)
			if got != tt.want {
				t.Errorf("applyMagicLinks() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyMagicLinksWithRepo(t *testing.T) {
	r := repo.NewRepository()
	// Add an entity
	err := r.AddEntity(&catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "my-comp",
		},
	})
	if err != nil {
		t.Fatalf("failed to add entity: %v", err)
	}

	sd := &storeData{repo: r}
	ctx := context.WithValue(context.Background(), ctxRefData, sd)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "existing entity",
			input: "see component:my-comp",
			want:  "see [component:my-comp](/ui/components/my-comp)",
		},
		{
			name:  "missing entity",
			input: "see component:missing",
			want:  "see component:missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyMagicLinks(ctx, tt.input)
			if got != tt.want {
				t.Errorf("applyMagicLinks() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkdownWithMagicLinks(t *testing.T) {
	ctx := context.Background()

	t.Run("no namespace", func(t *testing.T) {
		input := "Please see component:my-comp for details."
		want := "<p>Please see <a href=\"/ui/components/my-comp\">component:my-comp</a> for details.</p>\n"
		got, err := markdownWithMagicLinks(ctx, input)
		if err != nil {
			t.Fatalf("markdownWithMagicLinks() error: %v", err)
		}
		if string(got) != want {
			t.Errorf("markdownWithMagicLinks() = %q, want %q", string(got), want)
		}
	})

	t.Run("with namespace", func(t *testing.T) {
		input := "Use api:ns1/my-api."
		want := "<p>Use <a href=\"/ui/apis/ns1%2Fmy-api\">api:ns1/my-api</a>.</p>\n"
		got, err := markdownWithMagicLinks(ctx, input)
		if err != nil {
			t.Fatalf("markdownWithMagicLinks() error: %v", err)
		}
		if string(got) != want {
			t.Errorf("markdownWithMagicLinks() = %q, want %q", string(got), want)
		}
	})
}
