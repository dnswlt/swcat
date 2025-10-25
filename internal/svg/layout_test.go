package svg

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
)

// Test helpers to create entities
func newComponent(name, systemName string) *catalog.Component {
	return &catalog.Component{
		Metadata: &catalog.Metadata{Name: name},
		Spec: &catalog.ComponentSpec{
			System: &catalog.Ref{Kind: catalog.KindSystem, Name: systemName},
		},
	}
}

func TestStandardLayouter_NodeContext(t *testing.T) {
	// Layouter with custom config for specific tests
	layouter := NewStandardLayouter(Config{
		ShowParentSystem: true,
		ShowAPIProvider:  true,
	})

	testCases := []struct {
		name           string
		layouter       *StandardLayouter
		entity         catalog.Entity
		contextEntity  catalog.Entity
		expectedLayout dot.NodeLayout
	}{
		{
			name:          "simple component with no context",
			layouter:      layouter,
			entity:        newComponent("my-comp", "my-sys"),
			contextEntity: nil,
			expectedLayout: dot.NodeLayout{
				Label:     "my-comp",
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
		{
			name:          "component in same system context",
			layouter:      layouter,
			entity:        newComponent("comp-a", "system-a"),
			contextEntity: newComponent("comp-b", "system-a"),
			expectedLayout: dot.NodeLayout{
				Label:     "comp-a",
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
		{
			name:          "component in different system context",
			layouter:      layouter,
			entity:        newComponent("comp-a", "system-a"),
			contextEntity: newComponent("comp-b", "system-b"),
			expectedLayout: dot.NodeLayout{
				Label:     PrefixNodeLabelSmall + "system-a\\n" + "comp-a",
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
		{
			name:     "component with stereotype annotation",
			layouter: layouter,
			entity: &catalog.Component{
				Metadata: &catalog.Metadata{
					Name:        "comp-with-stereotype",
					Annotations: map[string]string{catalog.AnnotSterotype: "custom-stereotype"},
				},
				Spec: &catalog.ComponentSpec{System: &catalog.Ref{Name: "my-sys"}},
			},
			contextEntity: nil,
			expectedLayout: dot.NodeLayout{
				Label:     PrefixNodeLabelEm + "&laquo;custom-stereotype&raquo;\\n" + "comp-with-stereotype",
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			layout := tc.layouter.NodeContext(tc.entity, tc.contextEntity)

			if layout.Label != tc.expectedLayout.Label {
				t.Errorf("unexpected label:\n  got: %q\n want: %q", layout.Label, tc.expectedLayout.Label)
			}
			if layout.FillColor != tc.expectedLayout.FillColor {
				t.Errorf("unexpected fill color: got %q, want %q", layout.FillColor, tc.expectedLayout.FillColor)
			}
			if layout.Shape != tc.expectedLayout.Shape {
				t.Errorf("unexpected shape: got %q, want %q", layout.Shape, tc.expectedLayout.Shape)
			}
		})
	}
}
