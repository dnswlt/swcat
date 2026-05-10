package svg

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/google/go-cmp/cmp"
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

func TestRenderer_nodeLayout(t *testing.T) {
	r := &render{Renderer: &Renderer{config: Config{ShowParentSystem: true}}}

	testCases := []struct {
		name           string
		entity         catalog.Entity
		contextEntity  catalog.Entity
		expectedLayout dot.NodeLayout
	}{
		{
			name:          "simple component with no context",
			entity:        newComponent("my-comp", "my-sys"),
			contextEntity: nil,
			expectedLayout: dot.NodeLayout{
				Labels:    []dot.NodeLabel{{Text: "my-comp"}},
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
		{
			name:          "component in same system context",
			entity:        newComponent("comp-a", "system-a"),
			contextEntity: newComponent("comp-b", "system-a"),
			expectedLayout: dot.NodeLayout{
				Labels:    []dot.NodeLabel{{Text: "comp-a"}},
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
		{
			name:          "component in different system context",
			entity:        newComponent("comp-a", "system-a"),
			contextEntity: newComponent("comp-b", "system-b"),
			expectedLayout: dot.NodeLayout{
				Labels: []dot.NodeLabel{
					{Text: "system-a", Style: dot.LSSmall | dot.LSLight},
					{Text: "comp-a"},
				},
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
		{
			name: "component with stereotype annotation",
			entity: &catalog.Component{
				Metadata: &catalog.Metadata{
					Name:        "comp-with-stereotype",
					Annotations: map[string]string{catalog.AnnotSterotype: "custom-stereotype"},
				},
				Spec: &catalog.ComponentSpec{System: &catalog.Ref{Name: "my-sys"}},
			},
			contextEntity: nil,
			expectedLayout: dot.NodeLayout{
				Labels: []dot.NodeLabel{
					{Text: "«custom-stereotype»", Style: dot.LSEm | dot.LSSmall | dot.LSLight},
					{Text: "comp-with-stereotype"},
				},
				FillColor: "#D2E5EF",
				Shape:     dot.NSRoundedBox,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			layout := r.nodeLayout(tc.entity, tc.contextEntity)

			if diff := cmp.Diff(tc.expectedLayout.Labels, layout.Labels); diff != "" {
				t.Errorf("unexpected labels (-want +got):\n%s", diff)
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
