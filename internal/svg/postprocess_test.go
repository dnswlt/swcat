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
			name:    "passes through normal text",
			input:   []byte(`<svg><g><text>just normal text</text></g></svg>`),
			want:    []byte(`<svg><g><text>just normal text</text></g></svg>`),
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

			if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(tc.want)) {
				t.Errorf("PostprocessSVG() =%swant =%s", string(got), string(tc.want))
			}
		})
	}
}
