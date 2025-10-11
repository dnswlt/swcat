package api

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

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
			name: "Record Style with Ambiguous Label should fail",
			yaml: `
ref:
  ref: component:default/api "API Ref"
  label: API Label
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
				t.Errorf("UnmarshalYAML() got = %v, want %v", wrapper.Ref, tc.want)
			}
		})
	}
}
