package svg

import (
	"bytes"
	"testing"
)

func TestPostprocessSVG(t *testing.T) {
	testCases := []struct {
		name    string
		input   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "happy path with prefixes",
			input:   []byte(`<svg><g><text>|. small text</text><text>|, emphatic text</text></g></svg>`),
			want:    []byte("<svg><g><text class=\"node-label-small\">small text</text><text class=\"node-label-em\">emphatic text</text></g></svg>"),
			wantErr: false,
		},
		{
			name:    "no prefixes",
			input:   []byte(`<svg><g><text>just normal text</text></g></svg>`),
			want:    []byte("<svg><g><text>just normal text</text></g></svg>"),
			wantErr: false,
		},
		{
			name:    "invalid prefix not in map",
			input:   []byte(`<svg><text>|3 invalid</text></svg>`),
			want:    []byte("<svg><text>|3 invalid</text></svg>"),
			wantErr: false,
		},
		{
			name:    "invalid prefix wrong format",
			input:   []byte(`<svg><text>|a invalid</text></svg>`),
			want:    []byte("<svg><text>|a invalid</text></svg>"),
			wantErr: false,
		},
		{
			name:    "prefix with no following text",
			input:   []byte(`<svg><text>|.</text></svg>`),
			want:    []byte("<svg><text class=\"node-label-small\"></text></svg>"),
			wantErr: false,
		},
		{
			name:    "prefix with only spaces after",
			input:   []byte(`<svg><text>|,   </text></svg>`),
			want:    []byte("<svg><text class=\"node-label-em\"></text></svg>"),
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   []byte(``),
			want:    []byte(``),
			wantErr: false,
		},
		{
			name:    "malformed xml",
			input:   []byte(`<svg><g>`),
			wantErr: true,
		},
		{
			name:    "text tag at end of file",
			input:   []byte(`<svg><text>|. at end</text></svg>`),
			want:    []byte("<svg><text class=\"node-label-small\">at end</text></svg>"),
			wantErr: false,
		},
		{
			name:    "self-closing text tag",
			input:   []byte(`<svg><text/></svg>`),
			want:    []byte(`<svg><text></text></svg>`),
			wantErr: false,
		},
		{
			name:    "filters out title elements",
			input:   []byte(`<svg><title>a title</title><g><text>some text</text></g></svg>`),
			want:    []byte(`<svg><g><text>some text</text></g></svg>`),
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PostprocessSVG(tc.input)

			if (err != nil) != tc.wantErr {
				t.Errorf("PostprocessSVG() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr {
				return
			}

			// Using TrimSpace on the byte slices provides a more robust comparison
			// that isn't sensitive to minor whitespace differences from the encoder.
			if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(tc.want)) {
				t.Errorf("PostprocessSVG() =%swant =%s", string(got), string(tc.want))
			}
		})
	}
}
