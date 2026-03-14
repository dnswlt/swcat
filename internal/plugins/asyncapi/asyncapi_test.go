package asyncapi

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseDispatchV3(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
info:
  title: Test V3 API
  version: 1.2.3
channels:
  ch1:
    address: addr1
    messages:
      msg1:
        $ref: '#/components/messages/msg1'
      msg2:
        name: message2
components:
  messages:
    msg1:
      name: message1
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "asyncapi_v3.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	parsedSpec, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsedSpec == nil {
		t.Fatal("Parse() returned nil spec")
	}

	if parsedSpec.Version() != "1.2.3" {
		t.Errorf("Expected version '1.2.3', got '%s'", parsedSpec.Version())
	}

	simple := parsedSpec.SimpleChannels()
	if len(simple) != 1 {
		t.Fatalf("Expected 1 simple channel, got %d", len(simple))
	}

	s := simple[0]
	if s.Name != "ch1" {
		t.Errorf("Expected name 'ch1', got '%s'", s.Name)
	}
	if s.Address != "addr1" {
		t.Errorf("Expected address 'addr1', got '%s'", s.Address)
	}

	slices.Sort(s.Messages)
	expectedMsgs := "msg1,msg2"
	actualMsgs := strings.Join(s.Messages, ",")
	if actualMsgs != expectedMsgs {
		t.Errorf("Expected messages '%s', got '%s'", expectedMsgs, actualMsgs)
	}
}

func TestParseDispatchV2(t *testing.T) {
	yamlContent := `
asyncapi: '2.6.0'
info:
  title: Test V2 API
  version: 2.3.4
channels:
  user/signedup:
    publish:
      message:
        name: UserSignedUp
    subscribe:
      message:
        title: inlineMsg
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "asyncapi_v2.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	parsedSpec, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsedSpec == nil {
		t.Fatal("Parse() returned nil spec")
	}

	if parsedSpec.Version() != "2.3.4" {
		t.Errorf("Expected version '2.3.4', got '%s'", parsedSpec.Version())
	}

	simple := parsedSpec.SimpleChannels()
	if len(simple) != 1 {
		t.Fatalf("Expected 1 simple channel, got %d", len(simple))
	}

	s := simple[0]
	// In V2, Name and Address are the same (the key)
	if s.Name != "user/signedup" {
		t.Errorf("Expected name 'user/signedup', got '%s'", s.Name)
	}
	if s.Address != "user/signedup" {
		t.Errorf("Expected address 'user/signedup', got '%s'", s.Address)
	}

	slices.Sort(s.Messages)
	expectedMsgs := "UserSignedUp,inlineMsg"
	actualMsgs := strings.Join(s.Messages, ",")
	if actualMsgs != expectedMsgs {
		t.Errorf("Expected messages '%s', got '%s'", expectedMsgs, actualMsgs)
	}
}

func TestParseAsyncAPIV2Adapter(t *testing.T) {
	yamlContent := `
asyncapi: '2.6.0'
info:
  title: Test V2 API
  version: 3.4.5
channels:
  user/signedup:
    publish:
      message:
        name: UserSignedUp
    subscribe:
      message:
        title: inlineMsg
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "asyncapi_v2.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	parsedSpec, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	
	if parsedSpec.Version() != "3.4.5" {
		t.Errorf("Expected version '3.4.5', got '%s'", parsedSpec.Version())
	}

	simple := parsedSpec.SimpleChannels()
	if len(simple) != 1 {
		t.Fatalf("Expected 1 simple channel, got %d", len(simple))
	}

	s := simple[0]
	// In V2, Name and Address are the same (the key)
	if s.Name != "user/signedup" {
		t.Errorf("Expected name 'user/signedup', got '%s'", s.Name)
	}
	if s.Address != "user/signedup" {
		t.Errorf("Expected address 'user/signedup', got '%s'", s.Address)
	}

	slices.Sort(s.Messages)
	expectedMsgs := "UserSignedUp,inlineMsg"
	actualMsgs := strings.Join(s.Messages, ",")
	if actualMsgs != expectedMsgs {
		t.Errorf("Expected messages '%s', got '%s'", expectedMsgs, actualMsgs)
	}
}
