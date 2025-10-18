package catalog

import (
	"testing"

	"github.com/dnswlt/swcat/internal/api"
	"gopkg.in/yaml.v3"
)

func TestCloneEntityFromAPI_Component(t *testing.T) {
	input := `
kind: Component
metadata:
  name: yankee
spec:
  type: service
  lifecycle: prod
  owner: team-x
  system: system	
`
	var node yaml.Node
	err := yaml.Unmarshal([]byte(input), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	var component api.Component
	if err := node.Decode(&component); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	component.SetSourceInfo(&api.SourceInfo{
		Node: &node,
	})
	c, err := NewEntityFromAPI(&component)
	if err != nil {
		t.Fatalf("NewEntityFromAPI failed: %v", err)
	}
	cpy, err := cloneEntityFromAPI[*api.Component](c)
	if err != nil {
		t.Fatalf("CloneEntityFromAPI failed: %v", err)
	}
	if !cpy.GetRef().Equal(c.GetRef()) {
		t.Errorf("Refs differ: got: %s want: %s", cpy.GetRef(), c.GetRef())
	}
	if cpy.GetSourceInfo() != c.GetSourceInfo() {
		t.Error("SourceInfo pointers differ")
	}
}
