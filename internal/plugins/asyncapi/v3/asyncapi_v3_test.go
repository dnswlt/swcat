package v3

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAsyncAPI(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
info:
  title: User Signed Up Example
  version: 1.0.0
channels:
  userSignedUp:
    address: user/signedup
    messages:
      userSignedUp:
        $ref: '#/components/messages/UserSignedUp'
  userSignedUpRef:
    $ref: '#/components/channels/userSignedUpComponent'
operations:
  sendUserSignedUp:
    action: send
    channel:
      $ref: '#/components/channels/userSignedUpComponent'
    messages:
      - $ref: '#/components/messages/UserSignedUp'
components:
  messages:
    UserSignedUp:
      name: UserSignedUp
      title: User Signed Up
      summary: Inform about a new user
      contentType: application/json
  channels:
    userSignedUpComponent:
      address: user/signedup/component
      messages:
        inlineMsg:
          name: inlineMsg
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "asyncapi.yaml")
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
	if len(spec.Channels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(spec.Channels))
	}

	// Direct channel with ref message
	ch1, ok := spec.Channels["userSignedUp"]
	if !ok {
		t.Fatal("Channel 'userSignedUp' not found")
	}
	if ch1.Address != "user/signedup" {
		t.Errorf("Expected address 'user/signedup', got '%s'", ch1.Address)
	}
	if len(ch1.Messages) != 1 {
		t.Errorf("Expected 1 message in ch1, got %d", len(ch1.Messages))
	}
	msg1, ok := ch1.Messages["userSignedUp"]
	if !ok {
		t.Fatal("Message 'userSignedUp' not found in ch1")
	}
	if msg1.Name != "UserSignedUp" {
		t.Errorf("Expected message name 'UserSignedUp', got '%s'", msg1.Name)
	}
	if msg1.Title != "User Signed Up" {
		t.Errorf("Expected message title 'User Signed Up', got '%s'", msg1.Title)
	}

	// Ref channel
	ch2, ok := spec.Channels["userSignedUpRef"]
	if !ok {
		t.Fatal("Channel 'userSignedUpRef' not found")
	}
	// Check if ref was resolved (Address should be from component)
	if ch2.Address != "user/signedup/component" {
		t.Errorf("Expected resolved address 'user/signedup/component', got '%s'", ch2.Address)
	}
	if len(ch2.Messages) != 1 {
		t.Errorf("Expected 1 message in ch2, got %d", len(ch2.Messages))
	}
	msg2, ok := ch2.Messages["inlineMsg"]
	if !ok {
		t.Fatal("Message 'inlineMsg' not found in ch2")
	}
	if msg2.Name != "inlineMsg" {
		t.Errorf("Expected message name 'inlineMsg', got '%s'", msg2.Name)
	}

	// Verify Operations
	if len(spec.Operations) != 1 {
		t.Errorf("Expected 1 operation, got %d", len(spec.Operations))
	}
	op, ok := spec.Operations["sendUserSignedUp"]
	if !ok {
		t.Fatal("Operation 'sendUserSignedUp' not found")
	}
	if op.Action != "send" {
		t.Errorf("Expected action 'send', got '%s'", op.Action)
	}

	// Verify Operation Channel Ref resolution
	if op.Channel == nil {
		t.Fatal("Operation Channel is nil")
	}
	if op.Channel.Address != "user/signedup/component" {
		t.Errorf("Expected resolved op channel address 'user/signedup/component', got '%s'", op.Channel.Address)
	}

	// Verify Operation Message Ref resolution
	if len(op.Messages) != 1 {
		t.Errorf("Expected 1 message in op, got %d", len(op.Messages))
	}
	opMsg := op.Messages[0]
	if opMsg.Name != "UserSignedUp" {
		t.Errorf("Expected op message name 'UserSignedUp', got '%s'", opMsg.Name)
	}
	if opMsg.Summary != "Inform about a new user" {
		t.Errorf("Expected op message summary 'Inform about a new user', got '%s'", opMsg.Summary)
	}
}
