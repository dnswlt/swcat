package svg

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestColorStringUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ColorString
		wantErr bool
	}{
		{"ValidHexLower", `"#abcdef"`, "#abcdef", false},
		{"ValidHexUpper", `"#ABCDEF"`, "#ABCDEF", false},
		{"ValidColorName", `"white"`, "white", false},
		{"MixedCaseColorName", `"Blue"`, "Blue", false},
		{"Whitespace", `"#abcdef "`, "", true},
		{"InvalidShortHex", `"#fff"`, "", true},
		{"InvalidCharInHex", `"#gggggg"`, "", true},
		{"NumberInColorName", `"red1"`, "", true},
		{"NonStringScalar", `123`, "", true}, // YAML will decode this as non-scalar
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c ColorString
			err := yaml.Unmarshal([]byte(tt.input), &c)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if !tt.wantErr && c != tt.want {
				t.Errorf("expected: %q, got: %q", tt.want, c)
			}
		})
	}
}
