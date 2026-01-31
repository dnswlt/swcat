package asyncapi

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type SimpleChannel struct {
	Address  string   `json:"address"`
	Messages []string `json:"messages"`
}

type Message struct {
	Ref         string     `yaml:"$ref,omitempty"`
	Name        string     `yaml:"name,omitempty"`
	Title       string     `yaml:"title,omitempty"`
	Summary     string     `yaml:"summary,omitempty"`
	Description string     `yaml:"description,omitempty"`
	ContentType string     `yaml:"contentType,omitempty"`
	Payload     *yaml.Node `yaml:"payload,omitempty"`
}

type Operation struct {
	Ref         string     `yaml:"$ref,omitempty"`
	Action      string     `yaml:"action,omitempty"`
	Channel     *Channel   `yaml:"channel,omitempty"`
	Messages    []*Message `yaml:"messages,omitempty"`
	Title       string     `yaml:"title,omitempty"`
	Summary     string     `yaml:"summary,omitempty"`
	Description string     `yaml:"description,omitempty"`
}

type Channel struct {
	Ref         string              `yaml:"$ref,omitempty"`
	Address     string              `yaml:"address,omitempty"`
	Messages    map[string]*Message `yaml:"messages,omitempty"`
	Title       string              `yaml:"title,omitempty"`
	Summary     string              `yaml:"summary,omitempty"`
	Description string              `yaml:"description,omitempty"`
}

type Components struct {
	Channels   map[string]*Channel   `yaml:"channels,omitempty"`
	Operations map[string]*Operation `yaml:"operations,omitempty"`
	Messages   map[string]*Message   `yaml:"messages,omitempty"`
	Schemas    map[string]*yaml.Node `yaml:"schemas,omitempty"`
}

type Spec struct {
	AsyncAPI   string                `yaml:"asyncapi"`
	Channels   map[string]*Channel   `yaml:"channels"`
	Operations map[string]*Operation `yaml:"operations"`
	Components *Components           `yaml:"components"`
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
	// Resolve Channels
	for k, ch := range s.Channels {
		if ch.Ref != "" {
			if name, found := strings.CutPrefix(ch.Ref, "#/components/channels/"); found {
				if resolved, ok := s.Components.Channels[name]; ok {
					s.Channels[k] = resolved
					ch = resolved
				}
			}
		}
		// Resolve Messages in Channel
		for mk, msg := range ch.Messages {
			if msg.Ref != "" {
				if name, found := strings.CutPrefix(msg.Ref, "#/components/messages/"); found {
					if resolved, ok := s.Components.Messages[name]; ok {
						ch.Messages[mk] = resolved
					}
				}
			}
		}
	}

	// Resolve Operations
	for k, op := range s.Operations {
		if op.Ref != "" {
			if name, found := strings.CutPrefix(op.Ref, "#/components/operations/"); found {
				if resolved, ok := s.Components.Operations[name]; ok {
					s.Operations[k] = resolved
					op = resolved
				}
			}
		}
		// Resolve Channel in Operation
		if op.Channel != nil && op.Channel.Ref != "" {
			if name, found := strings.CutPrefix(op.Channel.Ref, "#/components/channels/"); found {
				if resolved, ok := s.Components.Channels[name]; ok {
					op.Channel = resolved
				}
			}
		}
		// Resolve Messages in Operation
		for mk, msg := range op.Messages {
			if msg.Ref != "" {
				if name, found := strings.CutPrefix(msg.Ref, "#/components/messages/"); found {
					if resolved, ok := s.Components.Messages[name]; ok {
						op.Messages[mk] = resolved
					}
				}
			}
		}
	}
}

// SimpleChannels returns a simplified view of the channels defined in the spec.
// If no channels are present, it returns an empty slice (not nil), for convenient JSON marshalling.
func (s *Spec) SimpleChannels() []*SimpleChannel {
	result := []*SimpleChannel{}
	for _, ch := range s.Channels {
		msgs := make([]string, 0, len(ch.Messages))
		for name := range ch.Messages {
			msgs = append(msgs, name)
		}
		result = append(result, &SimpleChannel{
			Address:  ch.Address,
			Messages: msgs,
		})
	}
	return result
}
