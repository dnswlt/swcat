package api

import "testing"

func TestNewEntityFromString(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: my-component
spec:
  type: service
  owner: my-group
  lifecycle: experimental
`
		entity, err := NewEntityFromString(content)
		if err != nil {
			t.Fatalf("NewEntityFromString() error = %v, wantErr %v", err, false)
		}
		if entity == nil {
			t.Fatal("entity is nil")
		}
		if component, ok := entity.(*Component); !ok {
			t.Fatalf("entity is not a *Component")
		} else {
			if component.Metadata.Name != "my-component" {
				t.Errorf("component.Metadata.Name = %s, want %s", component.Metadata.Name, "my-component")
			}
			if component.Spec.Type != "service" {
				t.Errorf("component.Spec.Type = %s, want %s", component.Spec.Type, "service")
			}
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: my-component
spec:
  type: service
  owner: my-group
  lifecycle: experimental
  foo: bar
`
		_, err := NewEntityFromString(content)
		if err == nil {
			t.Errorf("NewEntityFromString() error = %v, wantErr %v", err, true)
		}
	})
}
