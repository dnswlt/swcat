package maven

import (
	"os"
	"path/filepath"
	"testing"
)

const typicalSettingsXML = `<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0
                              https://maven.apache.org/xsd/settings-1.0.0.xsd">
  <servers>
    <server>
      <id>central</id>
      <username>alice</username>
      <password>s3cr3t</password>
    </server>
    <server>
      <id>my-artifactory</id>
      <username>bob</username>
      <password>hunter2</password>
      <privateKey>/home/bob/.ssh/id_rsa</privateKey>
      <passphrase>keypass</passphrase>
    </server>
  </servers>
</settings>
`

func writeSettingsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.xml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write settings file: %v", err)
	}
	return path
}

func TestReadSettings_Servers(t *testing.T) {
	path := writeSettingsFile(t, typicalSettingsXML)
	s, err := ReadSettings(path)
	if err != nil {
		t.Fatalf("ReadSettings() error = %v", err)
	}
	if len(s.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(s.Servers))
	}

	tests := []struct {
		id       string
		username string
		password string
		privKey  string
		pass     string
	}{
		{"central", "alice", "s3cr3t", "", ""},
		{"my-artifactory", "bob", "hunter2", "/home/bob/.ssh/id_rsa", "keypass"},
	}
	for i, tt := range tests {
		srv := s.Servers[i]
		if srv.ID != tt.id {
			t.Errorf("server[%d].ID = %q, want %q", i, srv.ID, tt.id)
		}
		if srv.Username != tt.username {
			t.Errorf("server[%d].Username = %q, want %q", i, srv.Username, tt.username)
		}
		if srv.Password != tt.password {
			t.Errorf("server[%d].Password = %q, want %q", i, srv.Password, tt.password)
		}
		if srv.PrivateKey != tt.privKey {
			t.Errorf("server[%d].PrivateKey = %q, want %q", i, srv.PrivateKey, tt.privKey)
		}
		if srv.Passphrase != tt.pass {
			t.Errorf("server[%d].Passphrase = %q, want %q", i, srv.Passphrase, tt.pass)
		}
	}
}

func TestSettings_ServerByID(t *testing.T) {
	path := writeSettingsFile(t, typicalSettingsXML)
	s, err := ReadSettings(path)
	if err != nil {
		t.Fatalf("ReadSettings() error = %v", err)
	}

	srv, err := s.ServerByID("my-artifactory")
	if err != nil {
		t.Fatalf("ServerByID() error = %v", err)
	}
	if srv.Username != "bob" {
		t.Errorf("Username = %q, want %q", srv.Username, "bob")
	}

	_, err = s.ServerByID("nonexistent")
	if err == nil {
		t.Error("ServerByID() expected error for unknown id, got nil")
	}
}
