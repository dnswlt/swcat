package v2

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Message struct {
	Ref         string     `yaml:"$ref,omitempty"`
	Name        string     `yaml:"name,omitempty"`
	Title       string     `yaml:"title,omitempty"`
	Summary     string     `yaml:"summary,omitempty"`
	ContentType string     `yaml:"contentType,omitempty"`
	Payload     *yaml.Node `yaml:"payload,omitempty"`
}

type Operation struct {
	Message *Message `yaml:"message,omitempty"`
}

type ChannelItem struct {
	Ref       string     `yaml:"$ref,omitempty"`
	Publish   *Operation `yaml:"publish,omitempty"`
	Subscribe *Operation `yaml:"subscribe,omitempty"`
}

type Components struct {
	Messages map[string]*Message `yaml:"messages,omitempty"`
}

type Spec struct {
	AsyncAPI   string                  `yaml:"asyncapi"`
	Channels   map[string]*ChannelItem `yaml:"channels"`
	Components *Components             `yaml:"components"`
}

func Parse(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML from %s: %w", path, err)
	}

	spec.Resolve()
	return &spec, nil
}

func (s *Spec) Resolve() {
	if s.Components == nil {
		return
	}

	for _, ch := range s.Channels {
		// Resolve messages in Publish operation
		if ch.Publish != nil && ch.Publish.Message != nil {
			s.resolveMessage(ch.Publish.Message)
		}
		// Resolve messages in Subscribe operation
		if ch.Subscribe != nil && ch.Subscribe.Message != nil {
			s.resolveMessage(ch.Subscribe.Message)
		}
	}
}

func (s *Spec) resolveMessage(msg *Message) {
	if msg.Ref != "" {
		if name, found := strings.CutPrefix(msg.Ref, "#/components/messages/"); found {
			if resolved, ok := s.Components.Messages[name]; ok {
				// Copy resolved fields to msg
				*msg = *resolved
			}
		}
	}
}
