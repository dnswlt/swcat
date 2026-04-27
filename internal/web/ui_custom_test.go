package web

import (
	"html/template"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/google/go-cmp/cmp"
)

func mustTemplate(t *testing.T, src string) *config.CustomContent {
	t.Helper()
	c, err := config.NewCustomContent(src)
	if err != nil {
		t.Fatalf("NewCustomContent: %v", err)
	}
	return c
}

func TestNewCustomContent_RawJSON(t *testing.T) {
	c := &config.CustomContent{Heading: "Raw", Open: true, Rank: 3}
	got, err := newCustomContent(c, `{"b":2,"a":1}`)
	if err != nil {
		t.Fatalf("newCustomContent: %v", err)
	}
	want := &CustomContent{
		Heading: "Raw",
		Open:    true,
		Rank:    3,
		Code:    "{\n  \"a\": 1,\n  \"b\": 2\n}",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewCustomContent_RawJSON_InvalidErrors(t *testing.T) {
	c := &config.CustomContent{Heading: "Raw"}
	if _, err := newCustomContent(c, `not json`); err == nil {
		t.Fatalf("expected error for non-JSON value, got nil")
	}
}

func TestNewCustomContent_TemplateOnObject(t *testing.T) {
	c := mustTemplate(t, `<p>{{ .name }} ({{ .role }})</p>`)
	c.Heading = "Person"
	got, err := newCustomContent(c, `{"name": "Alice", "role": "admin"}`)
	if err != nil {
		t.Fatalf("newCustomContent: %v", err)
	}
	if got.Code != "" {
		t.Errorf("Code should be empty when template is set, got %q", got.Code)
	}
	want := template.HTML(`<p>Alice (admin)</p>`)
	if got.HTML != want {
		t.Errorf("HTML = %q, want %q", got.HTML, want)
	}
}

func TestNewCustomContent_TemplateOnArray(t *testing.T) {
	c := mustTemplate(t, `<ul>{{ range . }}<li>{{ . }}</li>{{ end }}</ul>`)
	got, err := newCustomContent(c, `["a","b","c"]`)
	if err != nil {
		t.Fatalf("newCustomContent: %v", err)
	}
	want := template.HTML(`<ul><li>a</li><li>b</li><li>c</li></ul>`)
	if got.HTML != want {
		t.Errorf("HTML = %q, want %q", got.HTML, want)
	}
}

func TestNewCustomContent_TemplateUsesJoinHelper(t *testing.T) {
	c := mustTemplate(t, `<span>{{ .messages | join }}</span>`)
	got, err := newCustomContent(c, `{"messages": ["one","two","three"]}`)
	if err != nil {
		t.Fatalf("newCustomContent: %v", err)
	}
	want := template.HTML(`<span>one, two, three</span>`)
	if got.HTML != want {
		t.Errorf("HTML = %q, want %q", got.HTML, want)
	}
}

func TestNewCustomContent_TemplateAutoEscapesData(t *testing.T) {
	// html/template must escape data even though template literals are raw HTML.
	c := mustTemplate(t, `<p>{{ . }}</p>`)
	got, err := newCustomContent(c, `"<script>alert(1)</script>"`)
	if err != nil {
		t.Fatalf("newCustomContent: %v", err)
	}
	want := template.HTML(`<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>`)
	if got.HTML != want {
		t.Errorf("HTML = %q, want %q", got.HTML, want)
	}
}

func TestNewCustomContent_TemplateNonJSONValueIsRawString(t *testing.T) {
	// When the value isn't valid JSON, the template gets the raw string as `.`.
	c := mustTemplate(t, `<p>{{ . }}</p>`)
	got, err := newCustomContent(c, `not really json`)
	if err != nil {
		t.Fatalf("newCustomContent: %v", err)
	}
	want := template.HTML(`<p>not really json</p>`)
	if got.HTML != want {
		t.Errorf("HTML = %q, want %q", got.HTML, want)
	}
}

func TestNewCustomContent_TemplateExecutionErrorPropagates(t *testing.T) {
	// range over a non-iterable value errors at execution time.
	c := mustTemplate(t, `{{ range . }}{{ end }}`)
	if _, err := newCustomContent(c, `42`); err == nil {
		t.Fatalf("expected template execution error, got nil")
	}
}

func TestNewCustomContentObservation_AddsMeta(t *testing.T) {
	c := mustTemplate(t, `<p>{{ . }}</p>`)
	c.Heading = "Status"
	updated := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	obs := catalog.Observation{
		Value:     []byte(`"hello"`),
		UpdatedAt: updated,
		Version:   "v1.2.3",
	}
	got, err := newCustomContentObservation(c, obs)
	if err != nil {
		t.Fatalf("newCustomContentObservation: %v", err)
	}
	want := &CustomContent{
		Heading: "Status",
		HTML:    template.HTML(`<p>hello</p>`),
		Meta: []ccAttr{
			{Name: "updatedAt", Value: "2026-04-26T10:00:00Z"},
			{Name: "version", Value: "v1.2.3"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewCustomContentObservation_NoVersion(t *testing.T) {
	c := &config.CustomContent{} // raw JSON view
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	obs := catalog.Observation{
		Value:     []byte(`{"x":1}`),
		UpdatedAt: updated,
	}
	got, err := newCustomContentObservation(c, obs)
	if err != nil {
		t.Fatalf("newCustomContentObservation: %v", err)
	}
	wantMeta := []ccAttr{{Name: "updatedAt", Value: "2026-01-02T03:04:05Z"}}
	if diff := cmp.Diff(wantMeta, got.Meta); diff != "" {
		t.Errorf("Meta mismatch (-want +got):\n%s", diff)
	}
}
