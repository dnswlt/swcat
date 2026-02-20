package maven

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

// Settings represents the relevant parts of a Maven ~/.m2/settings.xml file.
type Settings struct {
	XMLName xml.Name `xml:"settings"`
	Servers []Server `xml:"servers>server"`
}

// Server represents a <server> entry in the Maven settings <servers> section.
type Server struct {
	ID          string `xml:"id"`
	Username    string `xml:"username"`
	Password    string `xml:"password"`
	PrivateKey  string `xml:"privateKey"`
	Passphrase  string `xml:"passphrase"`
}

// ReadSettings reads and parses a Maven settings.xml file.
// If path is empty, it defaults to ~/.m2/settings.xml.
func ReadSettings(path string) (*Settings, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("maven settings: cannot determine home directory: %w", err)
		}
		path = filepath.Join(home, ".m2", "settings.xml")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("maven settings: cannot read %s: %w", path, err)
	}
	var s Settings
	if err := xml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("maven settings: cannot parse %s: %w", path, err)
	}
	return &s, nil
}

// ServerByID returns the Server with the given id, or an error if not found.
func (s *Settings) ServerByID(id string) (*Server, error) {
	for i := range s.Servers {
		if s.Servers[i].ID == id {
			return &s.Servers[i], nil
		}
	}
	return nil, fmt.Errorf("maven settings: no server with id %q", id)
}
