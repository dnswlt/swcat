package web

import (
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/google/go-cmp/cmp"
	"github.com/tidwall/gjson"
)

func TestGJSONPath(t *testing.T) {

	r := gjson.Get(`{"some": {"thing": "foo"}}`, "some.thing")
	if r.String() != "foo" {
		t.Fatalf("Want \"foo\", got %v", r)
	}
	// String() on objects does not format.
	r = gjson.Get(`{"some": {"thing":  "foo"}}`, "some")
	want := `{"thing":  "foo"}`
	if got := r.String(); got != want {
		t.Fatalf("Want %q, got %q", want, got)
	}

	r = gjson.Get(`{"top": {"list": [{"value": "foo"}, {"value": "bar"}]}}`, "top.list.#.value")
	if want := `["foo","bar"]`; r.Raw != want {
		t.Fatalf("Want %q, got %q", want, r.Raw)
	}

	// Invalid JSON
	r = gjson.Get(`{invalid_json \ foo`, "yankee")
	if r.Raw != "" {
		t.Fatalf("Unexpected Raw value for invalid JSON: %q", r.Raw)
	}

	// Invalid Path
	r = gjson.Get(`{"some": {"thing": "foo"}}`, `\begin{document}`)
	if r.Raw != "" {
		t.Fatalf("Unexpected Raw value for invalid JSON: %q", r.Raw)
	}

}

func mustColumn(t *testing.T, header, data string) *config.CustomColumn {
	t.Helper()
	c, err := config.NewCustomColumn(header, data)
	if err != nil {
		t.Fatalf("NewCustomColumn(%q, %q): %v", header, data, err)
	}
	return c
}

func TestNewCustomContent(t *testing.T) {
	tests := []struct {
		name    string
		ccc     config.CustomContent
		content string
		want    *CustomContent
		wantErr bool
	}{
		{
			name:    "text style",
			ccc:     config.CustomContent{Heading: "My Text", Style: "text"},
			content: `"Hello World"`,
			want:    &CustomContent{Heading: "My Text", Text: "Hello World"},
		},
		{
			name:    "list style valid",
			ccc:     config.CustomContent{Heading: "My List", Style: "list"},
			content: `["a", "b"]`,
			want:    &CustomContent{Heading: "My List", Items: []string{"a", "b"}},
		},
		{
			name:    "json style valid",
			ccc:     config.CustomContent{Heading: "My JSON", Style: "json"},
			content: `{"key": "value"}`,
			want:    &CustomContent{Heading: "My JSON", Code: "{\n  \"key\": \"value\"\n}"},
		},
		{
			name:    "attrs style valid",
			ccc:     config.CustomContent{Heading: "My Attrs", Style: "attrs"},
			content: `{"b": 2, "a": "1"}`,
			want: &CustomContent{
				Heading: "My Attrs",
				Attrs: []ccAttr{
					{Name: "a", Value: "1"},
					{Name: "b", Value: "2"},
				},
			},
		},
		{
			name: "table style valid",
			ccc: config.CustomContent{
				Heading: "My Table",
				Style:   "table",
				Columns: []*config.CustomColumn{
					mustColumn(t, "Name", "{{.name}}"),
					mustColumn(t, "Age", "{{.age}}"),
				},
			},
			content: `[{"name":"alice","age":30},{"name":"bob","age":25}]`,
			want: &CustomContent{
				Heading: "My Table",
				Table: &ccTable{
					Headers: []string{"Name", "Age"},
					Rows: []*ccRow{
						{Columns: []string{"alice", "30"}},
						{Columns: []string{"bob", "25"}},
					},
				},
			},
		},
		{
			name:    "selector extracts sub-value",
			ccc:     config.CustomContent{Heading: "Sel", Style: "text", Selector: "msg"},
			content: `{"msg": "hi", "other": 1}`,
			want:    &CustomContent{Heading: "Sel", Text: "hi"},
		},
		{
			name:    "selector matches nothing",
			ccc:     config.CustomContent{Heading: "Sel", Style: "text", Selector: "missing"},
			content: `{"msg": "hi"}`,
			wantErr: true,
		},
		{
			name:    "attrs style invalid json",
			ccc:     config.CustomContent{Heading: "My Attrs", Style: "attrs"},
			content: `invalid`,
			wantErr: true,
		},
		{
			name:    "unknown style",
			ccc:     config.CustomContent{Heading: "Unknown", Style: "unknown"},
			content: "foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newCustomContent(&tt.ccc, tt.content)
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

func TestSetCCText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"json string unquoted", `"hello"`, "hello"},
		{"json number", `42`, "42"},
		{"json bool", `true`, "true"},
		{"json null", `null`, "null"},
		{"json object as compact json", `{"a":1}`, `{"a":1}`},
		{"json array as compact json", `[1,2]`, `[1,2]`},
		{"invalid json passed through", `not json`, "not json"},
		{"empty string passed through", ``, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CustomContent{}
			setCCText(cc, tt.value)
			if cc.Text != tt.want {
				t.Errorf("Text = %q, want %q", cc.Text, tt.want)
			}
		})
	}
}

func TestSetCCList(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{
			name:  "array of strings",
			value: `["a","b"]`,
			want:  []string{"a", "b"},
		},
		{
			name:  "mixed types tolerated",
			value: `["a", 1, true, null, {"k":"v"}, [1,2]]`,
			want:  []string{"a", "1", "true", "null", `{"k":"v"}`, `[1,2]`},
		},
		{
			name:  "empty array",
			value: `[]`,
			want:  []string{},
		},
		{
			name:    "not an array",
			value:   `{"a":1}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			value:   `nope`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CustomContent{}
			err := setCCList(cc, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(tt.want, cc.Items); diff != "" {
				t.Errorf("Items mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetCCTable(t *testing.T) {
	nameCol := mustColumn(t, "Name", "{{.name}}")
	ageCol := mustColumn(t, "Age", "{{.age}}")
	noHeaderCol := mustColumn(t, "", "{{.name}}")
	missingFieldCol := mustColumn(t, "Missing", "{{.nope}}")

	tests := []struct {
		name    string
		columns []*config.CustomColumn
		value   string
		want    *ccTable
		wantErr bool
	}{
		{
			name:    "with headers",
			columns: []*config.CustomColumn{nameCol, ageCol},
			value:   `[{"name":"alice","age":30},{"name":"bob","age":25}]`,
			want: &ccTable{
				Headers: []string{"Name", "Age"},
				Rows: []*ccRow{
					{Columns: []string{"alice", "30"}},
					{Columns: []string{"bob", "25"}},
				},
			},
		},
		{
			name:    "no headers when all column headers empty",
			columns: []*config.CustomColumn{noHeaderCol},
			value:   `[{"name":"x"}]`,
			want: &ccTable{
				Rows: []*ccRow{
					{Columns: []string{"x"}},
				},
			},
		},
		{
			name:    "missing field renders empty",
			columns: []*config.CustomColumn{missingFieldCol},
			value:   `[{"name":"x"}]`,
			want: &ccTable{
				Headers: []string{"Missing"},
				Rows: []*ccRow{
					{Columns: []string{"<no value>"}},
				},
			},
		},
		{
			name:    "empty array yields empty rows",
			columns: []*config.CustomColumn{nameCol},
			value:   `[]`,
			want: &ccTable{
				Headers: []string{"Name"},
				Rows:    []*ccRow{},
			},
		},
		{
			name:    "not an array of objects",
			columns: []*config.CustomColumn{nameCol},
			value:   `"nope"`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			columns: []*config.CustomColumn{nameCol},
			value:   `bad`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CustomContent{}
			err := setCCTable(cc, tt.columns, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(tt.want, cc.Table); diff != "" {
				t.Errorf("Table mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetCCCode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{
			name:  "object is indented",
			value: `{"a":1,"b":2}`,
			want:  "{\n  \"a\": 1,\n  \"b\": 2\n}",
		},
		{
			name:  "array is indented",
			value: `[1,2]`,
			want:  "[\n  1,\n  2\n]",
		},
		{
			name:    "invalid json errors",
			value:   `not json`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CustomContent{}
			err := setCCCode(cc, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if cc.Code != tt.want {
				t.Errorf("Code = %q, want %q", cc.Code, tt.want)
			}
		})
	}
}

func TestSetCCAttrs(t *testing.T) {
	tests := []struct {
		name    string
		fields  []string
		value   string
		want    []ccAttr
		wantErr bool
	}{
		{
			name:   "no fields dumps all sorted",
			fields: nil,
			value:  `{"b":2,"a":"1"}`,
			want: []ccAttr{
				{Name: "a", Value: "1"},
				{Name: "b", Value: "2"},
			},
		},
		{
			name:   "nested values rendered as JSON",
			fields: nil,
			value:  `{"obj":{"x":1},"arr":[1,2]}`,
			want: []ccAttr{
				{Name: "arr", Value: "[1,2]"},
				{Name: "obj", Value: `{"x":1}`},
			},
		},
		{
			name:    "no fields, invalid json errors",
			fields:  nil,
			value:   `not json`,
			wantErr: true,
		},
		{
			name:   "with fields uses gjson paths",
			fields: []string{"a", "nested.b", "missing"},
			value:  `{"a":"one","nested":{"b":"two"}}`,
			want: []ccAttr{
				{Name: "a", Value: "one"},
				{Name: "nested.b", Value: "two"},
			},
		},
		{
			name:    "with fields, invalid json errors",
			fields:  []string{"a"},
			value:   `not json`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CustomContent{}
			err := setCCAttrs(cc, tt.fields, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(tt.want, cc.Attrs); diff != "" {
				t.Errorf("Attrs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatJSONValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"string unquoted", "hi", "hi"},
		{"number", float64(3), "3"},
		{"bool", true, "true"},
		{"nil as null", nil, "null"},
		{"map as json", map[string]any{"k": "v"}, `{"k":"v"}`},
		{"slice as json", []any{1.0, "a"}, `[1,"a"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatJSONValue(tt.in); got != tt.want {
				t.Errorf("formatJSONValue(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewCustomContentGroup_InlineFallback(t *testing.T) {
	// When Blocks is empty, the inline CustomContent is rendered as a single
	// block, and the inline Heading is consumed as the group heading — the
	// block itself must have an empty Heading.
	s := &config.CustomContentSection{
		Rank: 5,
		Open: true,
		CustomContent: config.CustomContent{
			Heading: "Inline Section",
			Style:   "text",
		},
	}
	got, err := newCustomContentGroup(s, `"hi"`)
	if err != nil {
		t.Fatalf("newCustomContentGroup: %v", err)
	}
	want := &CustomContentGroup{
		Heading: "Inline Section",
		Open:    true,
		Rank:    5,
		Blocks: []*CustomContent{
			{Text: "hi"}, // No sub-heading.
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewCustomContentGroup_Blocks(t *testing.T) {
	// Multiple blocks with selectors slicing the same JSON value. Each
	// block's Heading is preserved as a sub-heading; the group's Heading
	// comes from the inline (outer) Heading.
	s := &config.CustomContentSection{
		Rank: 1,
		CustomContent: config.CustomContent{
			Heading: "Org Data",
		},
		Blocks: []*config.CustomContent{
			{Heading: "Team", Style: "attrs", Selector: "team"},
			{Heading: "Cost", Style: "text", Selector: "cost"},
			{Style: "json"}, // No sub-heading; renders the full value.
		},
	}
	value := `{"team": {"name": "Alpha"}, "cost": "12345"}`
	got, err := newCustomContentGroup(s, value)
	if err != nil {
		t.Fatalf("newCustomContentGroup: %v", err)
	}
	want := &CustomContentGroup{
		Heading: "Org Data",
		Rank:    1,
		Blocks: []*CustomContent{
			{Heading: "Team", Attrs: []ccAttr{{Name: "name", Value: "Alpha"}}},
			{Heading: "Cost", Text: "12345"},
			{Code: "{\n  \"cost\": \"12345\",\n  \"team\": {\n    \"name\": \"Alpha\"\n  }\n}"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewCustomContentGroup_BlockErrorPropagates(t *testing.T) {
	s := &config.CustomContentSection{
		CustomContent: config.CustomContent{Heading: "Sec"},
		Blocks: []*config.CustomContent{
			{Style: "text", Selector: "ok"},
			{Style: "text", Selector: "missing"}, // Will error.
		},
	}
	if _, err := newCustomContentGroup(s, `{"ok": "fine"}`); err == nil {
		t.Fatalf("expected error for missing selector, got nil")
	}
}

func TestNewCustomContentGroupObservation_AddsMeta(t *testing.T) {
	updated := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	obs := catalog.Observation{
		Value:     []byte(`"hello"`),
		UpdatedAt: updated,
		Version:   "v1.2.3",
	}
	s := &config.CustomContentSection{
		CustomContent: config.CustomContent{Heading: "Status", Style: "text"},
	}
	got, err := newCustomContentGroupObservation(s, obs)
	if err != nil {
		t.Fatalf("newCustomContentGroupObservation: %v", err)
	}
	want := &CustomContentGroup{
		Heading: "Status",
		Blocks:  []*CustomContent{{Text: "hello"}},
		Meta: []ccAttr{
			{Name: "updatedAt", Value: "2026-04-26T10:00:00Z"},
			{Name: "version", Value: "v1.2.3"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewCustomContentGroupObservation_NoVersion(t *testing.T) {
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	obs := catalog.Observation{
		Value:     []byte(`"x"`),
		UpdatedAt: updated,
	}
	s := &config.CustomContentSection{
		CustomContent: config.CustomContent{Style: "text"},
	}
	got, err := newCustomContentGroupObservation(s, obs)
	if err != nil {
		t.Fatalf("newCustomContentGroupObservation: %v", err)
	}
	wantMeta := []ccAttr{{Name: "updatedAt", Value: "2026-01-02T03:04:05Z"}}
	if diff := cmp.Diff(wantMeta, got.Meta); diff != "" {
		t.Errorf("Meta mismatch (-want +got):\n%s", diff)
	}
}
