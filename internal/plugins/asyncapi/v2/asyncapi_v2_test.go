package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAsyncAPIV2(t *testing.T) {
	yamlContent := `
asyncapi: '2.6.0'
channels:
  user/signedup:
    publish:
      message:
        $ref: '#/components/messages/UserSignedUp'
    subscribe:
      message:
        name: inlineMsg
components:
  messages:
    UserSignedUp:
      name: UserSignedUp
      title: User Signed Up
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "asyncapi_v2.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	spec, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if spec == nil {
		t.Fatal("Parse() returned nil spec")
	}

	// Verify Channels
	if len(spec.Channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(spec.Channels))
	}

	ch, ok := spec.Channels["user/signedup"]
	if !ok {
		t.Fatal("Channel 'user/signedup' not found")
	}

	// Verify Publish Message Resolution
	if ch.Publish == nil || ch.Publish.Message == nil {
		t.Fatal("Publish message is nil")
	}
	if ch.Publish.Message.Name != "UserSignedUp" {
		t.Errorf("Expected publish message name 'UserSignedUp', got '%s'", ch.Publish.Message.Name)
	}

	// Verify Subscribe Message
	if ch.Subscribe == nil || ch.Subscribe.Message == nil {
		t.Fatal("Subscribe message is nil")
	}
	if ch.Subscribe.Message.Name != "inlineMsg" {
		t.Errorf("Expected subscribe message name 'inlineMsg', got '%s'", ch.Subscribe.Message.Name)
	}
}
