package api

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

func TestParseLabelRef(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    *LabelRef
		wantErr bool
	}{
		{
			name:  "name only",
			input: "my-name",
			want: &LabelRef{
				Ref: &Ref{Kind: "", Namespace: DefaultNamespace, Name: "my-name"},
			},
		},
		{
			name:  "namespace and name",
			input: "my-namespace/my-name",
			want: &LabelRef{
				Ref: &Ref{Kind: "", Namespace: "my-namespace", Name: "my-name"},
			},
		},
		{
			name:  "kind, namespace, and name",
			input: "api:my-namespace/my-name",
			want: &LabelRef{
				Ref: &Ref{Kind: "api", Namespace: "my-namespace", Name: "my-name"},
			},
		},
		{
			name:  "ref with leading/trailing space",
			input: "  api:my-namespace/my-name  ",
			want: &LabelRef{
				Ref: &Ref{Kind: "api", Namespace: "my-namespace", Name: "my-name"},
			},
		},
		{
			name:  "ref with attached version",
			input: "foo/bar@v1",
			want: &LabelRef{
				Ref:   &Ref{Kind: "", Namespace: "foo", Name: "bar"},
				Attrs: map[string]string{VersionAttrKey: "v1"},
			},
		},
		{
			name:  "ref with spaced version",
			input: "foo/bar @v1",
			want: &LabelRef{
				Ref:   &Ref{Kind: "", Namespace: "foo", Name: "bar"},
				Attrs: map[string]string{VersionAttrKey: "v1"},
			},
		},
		{
			name:  "ref with label",
			input: `foo/bar "yankee"`,
			want: &LabelRef{
				Ref:   &Ref{Kind: "", Namespace: "foo", Name: "bar"},
				Label: "yankee",
			},
		},
		{
			name:  "ref with attached version and label",
			input: `api:foo/bar@v2 "yankee"`,
			want: &LabelRef{
				Ref:   &Ref{Kind: "api", Namespace: "foo", Name: "bar"},
				Attrs: map[string]string{VersionAttrKey: "v2"},
				Label: "yankee",
			},
		},
		{
			name:  "ref with spaced version and label",
			input: `api:foo/bar @v1 "yankee"`,
			want: &LabelRef{
				Ref:   &Ref{Kind: "api", Namespace: "foo", Name: "bar"},
				Attrs: map[string]string{VersionAttrKey: "v1"},
				Label: "yankee",
			},
		},
		{
			name:  "empty label",
			input: `foo/bar ""`,
			want: &LabelRef{
				Ref:   &Ref{Kind: "", Namespace: "foo", Name: "bar"},
				Label: "",
			},
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "version only",
			input:   "@v1",
			wantErr: true,
		},
		{
			name:    "label only",
			input:   `"yankee"`,
			wantErr: true,
		},
		{
			name:    "empty version string",
			input:   "foo/bar @",
			wantErr: true,
		},
		{
			name:    "unclosed label",
			input:   `foo/bar "yankee`,
			wantErr: true,
		},
		{
			name:    "trailing garbage",
			input:   `foo/bar "yankee" oops`,
			wantErr: true,
		},
		{
			name:    "misplaced version after label",
			input:   `foo/bar "yankee" @v1`,
			wantErr: true,
		},
		{
			name:    "multiple versions is an error",
			input:   "foo/bar@v2 @v1",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLabelRef(tc.input)

			if (err != nil) != tc.wantErr {
				t.Fatalf("parseLabelRef() error = %v, wantErr %v", err, tc.wantErr)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("parseLabelRef() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEntityRef_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name    string
		yaml    string
		want    Ref
		wantErr bool
	}{
		{
			name: "Simple String with Namespace",
			yaml: `ref: component:my-namespace/my-component`,
			want: Ref{Kind: "component", Namespace: "my-namespace", Name: "my-component"},
		},
		{
			name: "Simple String without Namespace",
			yaml: `ref: resource:my-resource`,
			want: Ref{Kind: "resource", Namespace: DefaultNamespace, Name: "my-resource"},
		},
		{
			name: "Invalid String Format (no colon)",
			yaml: `ref: my-component`,
			want: Ref{Kind: "", Namespace: DefaultNamespace, Name: "my-component"},
		},
		{
			name:    "String with Label should fail",
			yaml:    `ref: resource:my-resource "a label"`,
			wantErr: true,
		},
		{
			name:    "Not a string node",
			yaml:    `ref: { foo: bar }`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wrapper struct {
				Ref Ref `yaml:"ref"`
			}
			err := yaml.Unmarshal([]byte(tc.yaml), &wrapper)
			if (err != nil) != tc.wantErr {
				t.Fatalf("UnmarshalYAML() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(wrapper.Ref, tc.want) {
				t.Errorf("UnmarshalYAML() got = %+v, want %+v", wrapper.Ref, tc.want)
			}
		})
	}
}

func TestLabelRef_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name    string
		yaml    string
		want    LabelRef
		wantErr bool
	}{
		{
			name: "Simple String without label",
			yaml: `ref: component:my-namespace/my-component`,
			want: LabelRef{
				Ref:   &Ref{Kind: "component", Namespace: "my-namespace", Name: "my-component"},
				Label: "",
			},
		},
		{
			name: "Simple String without label",
			yaml: `ref: my-component`,
			want: LabelRef{
				Ref:   &Ref{Kind: "", Namespace: DefaultNamespace, Name: "my-component"},
				Label: "",
			},
		},
		{
			name: "String with Quoted Label",
			yaml: `ref: component:default/frontend "Main Web App"`,
			want: LabelRef{
				Ref:   &Ref{Kind: "component", Namespace: "default", Name: "frontend"},
				Label: "Main Web App",
			},
		},
		{
			name: "String with Version",
			yaml: `ref: component:default/frontend @v1.2`,
			want: LabelRef{
				Ref:   &Ref{Kind: "component", Namespace: "default", Name: "frontend"},
				Attrs: map[string]string{VersionAttrKey: "v1.2"},
			},
		},
		{
			name: "String with Version (no space)",
			yaml: `ref: component:default/frontend@v1.2`,
			want: LabelRef{
				Ref:   &Ref{Kind: "component", Namespace: "default", Name: "frontend"},
				Attrs: map[string]string{VersionAttrKey: "v1.2"},
			},
		},
		{
			name: "String with Version and Label",
			yaml: `ref: component:default/frontend @v1.2 "foo"`,
			want: LabelRef{
				Ref:   &Ref{Kind: "component", Namespace: "default", Name: "frontend"},
				Label: "foo",
				Attrs: map[string]string{VersionAttrKey: "v1.2"},
			},
		},
		{
			name: "Record Style with Namespace",
			yaml: `
ref:
  ref: component:production/api-gateway
  label: Main API Gateway
`,
			want: LabelRef{
				Ref:   &Ref{Kind: "component", Namespace: "production", Name: "api-gateway"},
				Label: "Main API Gateway",
			},
		},
		{
			name: "Record Style with Attrs",
			yaml: `
ref:
  ref: api-gateway
  label: Main API Gateway
  attrs:
    foo: bar
`,
			want: LabelRef{
				Ref:   &Ref{Name: "api-gateway"},
				Label: "Main API Gateway",
				Attrs: map[string]string{
					"foo": "bar",
				},
			},
		},
		{
			name: "Record Style without Namespace",
			yaml: `
ref:
  ref: group:jane-doe
  label: Jane Doe's Group Record
`,
			want: LabelRef{
				Ref:   &Ref{Kind: "group", Namespace: DefaultNamespace, Name: "jane-doe"},
				Label: "Jane Doe's Group Record",
			},
		},
		{
			name:    "String with Unquoted Label should fail",
			yaml:    `ref: group:jane-doe Jane Doe's Record`,
			wantErr: true,
		},
		{
			name:    "String with Brace-Enclosed Label should fail",
			yaml:    `ref: component:prod/api {API Gateway}`,
			wantErr: true,
		},
		{
			name:    "String with Label before Version should fail",
			yaml:    `ref: component:prod/api "foo" @v1`,
			wantErr: true,
		},
		{
			name: "Record Style with Ambiguous Label should fail",
			yaml: `
ref:
  ref: component:default/api "API Ref"
  label: API Label
`,
			wantErr: true,
		},
		{
			name: "Record Style with invalid field should fail",
			yaml: `
ref:
  ref: component:default/api
  labelxxx: API Label
`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wrapper struct {
				Ref LabelRef `yaml:"ref"`
			}
			err := yaml.Unmarshal([]byte(tc.yaml), &wrapper)
			if (err != nil) != tc.wantErr {
				t.Fatalf("UnmarshalYAML() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && wrapper.Ref.String() != tc.want.String() {
				t.Errorf("UnmarshalYAML() got = %v, want %v", wrapper.Ref.String(), tc.want.String())
			}
		})
	}
}
