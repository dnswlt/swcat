package lint

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestCheckLintAnnotation(t *testing.T) {
	config := &Config{
		CustomRules: []CustomRule{
			{
				Name:     "check-my-annotation",
				Severity: SeverityWarn,
				Func:     "lintAnnotation",
				Params: map[string]string{
					"annotation": "my/lint-finding",
				},
			},
		},
	}

	linter, err := NewLinter(config, KnownCustomChecks)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}

	tests := []struct {
		name         string
		annotations  map[string]string
		wantFindings int
		wantMessage  string
	}{
		{
			"no annotation",
			nil,
			0,
			"",
		},
		{
			"plain text annotation",
			map[string]string{
				"my/lint-finding": "Something is wrong",
			},
			1,
			"Something is wrong",
		},
		{
			"JSON finding annotation",
			map[string]string{
				"my/lint-finding": `{"message": "Structured error"}`,
			},
			1,
			"Structured error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &catalog.Component{
				Metadata: &catalog.Metadata{
					Name:        "test",
					Annotations: tt.annotations,
				},
				Spec: &catalog.ComponentSpec{Type: "service"},
			}
			pb := catalog.ToPB(comp)
			findings := linter.Lint(pb)

			if len(findings) != tt.wantFindings {
				t.Fatalf("got %d findings, want %d", len(findings), tt.wantFindings)
			}
			if tt.wantFindings > 0 {
				if findings[0].Message != tt.wantMessage {
					t.Errorf("got message %q, want %q", findings[0].Message, tt.wantMessage)
				}
				if findings[0].RuleName != "check-my-annotation" {
					t.Errorf("got rule %q, want %q", findings[0].RuleName, "check-my-annotation")
				}
				if findings[0].Severity != SeverityWarn {
					t.Errorf("got severity %q, want %q", findings[0].Severity, SeverityWarn)
				}
			}
		})
	}
}
