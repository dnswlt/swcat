// Package asyncapi parses AsyncAPI specifications of different versions
// (v2.x and v3.x) into a single, simplified representation.
package asyncapi

import (
	"fmt"
	"os"
	"slices"
	"strings"

	v2 "github.com/dnswlt/swcat/internal/plugins/asyncapi/v2"
	v3 "github.com/dnswlt/swcat/internal/plugins/asyncapi/v3"
	"gopkg.in/yaml.v3"
)

// SimpleChannel is a simplified representation of an AsyncAPI v2.x channel.
// In v2.x the channel is the container for its operations, so the channel is
// the natural unit to report.
type SimpleChannel struct {
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	Messages []string `json:"messages"`
}

// SimpleOperation is a simplified representation of an AsyncAPI v3.x operation.
// In v3.x operations reference channels rather than being nested inside them,
// so the operation is the natural unit to report.
type SimpleOperation struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Address string `json:"address"`
	// Ref is set only when the operation is a $ref we could not follow, in
	// which case the other fields are empty and nothing is known about how
	// the operation communicates. Renderers should say so rather than
	// reporting it as one-way.
	Ref string `json:"ref,omitempty"`
	// Reply is true if the operation declares a reply, i.e. it is a
	// request/reply operation rather than a fire-and-forget publish.
	Reply    bool     `json:"reply,omitempty"`
	Messages []string `json:"messages"`
}

// Extract is the result of parsing an AsyncAPI spec of any supported version.
//
// v2.x and v3.x model things differently: v2.x nests operations inside
// channels, while v3.x has operations reference channels many-to-one. Rather
// than projecting one onto the other and losing information in the process,
// each version is reported in its native shape: Channels is populated for
// v2.x specs, Operations for v3.x specs.
type Extract struct {
	AsyncAPIVersion string             `json:"asyncapiVersion"`
	Channels        []*SimpleChannel   `json:"channels,omitempty"`
	Operations      []*SimpleOperation `json:"operations,omitempty"`
}

// Parse reads an AsyncAPI specification from the given path, detects its
// version, and returns the simplified Extract.
func Parse(path string) (*Extract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return ParseBytes(data)
}

// ParseBytes parses an AsyncAPI specification from raw bytes, detects its
// version, and returns the simplified Extract.
func ParseBytes(data []byte) (*Extract, error) {
	// Detect version from the "asyncapi" root field.
	var meta struct {
		AsyncAPI string `yaml:"asyncapi"`
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML header: %w", err)
	}

	if strings.HasPrefix(meta.AsyncAPI, "2.") {
		spec, err := v2.ParseBytes(data)
		if err != nil {
			return nil, err
		}
		return extractV2(spec), nil
	}

	if strings.HasPrefix(meta.AsyncAPI, "3.") || meta.AsyncAPI == "3" {
		spec, err := v3.ParseBytes(data)
		if err != nil {
			return nil, err
		}
		return extractV3(spec), nil
	}

	return nil, fmt.Errorf("unsupported AsyncAPI version %q: expected version 2.x or 3.x", meta.AsyncAPI)
}

// extractV2 reports a v2.x spec as a list of channels.
func extractV2(spec *v2.Spec) *Extract {
	channels := []*SimpleChannel{}
	for addr, ch := range spec.Channels {
		// A key with no body under it parses as a nil entry; there is nothing
		// to report for it.
		if ch == nil {
			continue
		}
		msgs := []string{}
		collectMsg := func(op *v2.Operation) {
			if op == nil || op.Message == nil {
				return
			}
			if name := messageName(op.Message.Name, op.Message.Title, ""); name != "" {
				msgs = append(msgs, name)
			}
		}

		collectMsg(ch.Publish)
		collectMsg(ch.Subscribe)

		// In v2.x, the map key is the address; there's no distinct logical name.
		channels = append(channels, &SimpleChannel{
			Name:     addr,
			Address:  addr,
			Messages: msgs,
		})
	}
	slices.SortFunc(channels, func(a, b *SimpleChannel) int {
		return strings.Compare(a.Name, b.Name)
	})
	return &Extract{
		AsyncAPIVersion: spec.AsyncAPI,
		Channels:        channels,
	}
}

// extractV3 reports a v3.x spec as a list of operations. Channels that no
// operation references are not reported: they describe a place with no
// declared application-level use.
func extractV3(spec *v3.Spec) *Extract {
	operations := []*SimpleOperation{}
	for name, op := range spec.Operations {
		if op == nil {
			continue
		}
		simple := &SimpleOperation{
			Name: name,
			// Resolve replaces a followed $ref with its target, so a Ref
			// still present here is one we could not resolve.
			Ref:      op.Ref,
			Action:   op.Action,
			Reply:    op.Reply != nil,
			Messages: []string{},
		}
		simple.Channel = op.ChannelName
		if op.Channel != nil {
			simple.Address = op.Channel.Address
		}
		// An operation may narrow the channel's messages to a subset. Omitting
		// the field means every message on the channel is in play, whereas an
		// explicitly empty list means none of them are — so test for presence,
		// not for length.
		if op.Messages != nil {
			for _, msg := range op.Messages {
				if msg == nil {
					continue
				}
				// An unresolvable $ref has no name to show; fall back to the
				// reference itself rather than dropping the message silently.
				if n := messageName(msg.Name, msg.Title, msg.Ref); n != "" {
					simple.Messages = append(simple.Messages, n)
				}
			}
		} else if op.Channel != nil {
			for key, msg := range op.Channel.Messages {
				if msg == nil {
					continue
				}
				simple.Messages = append(simple.Messages, messageName(msg.Name, msg.Title, key))
			}
		}
		slices.Sort(simple.Messages)
		operations = append(operations, simple)
	}
	slices.SortFunc(operations, func(a, b *SimpleOperation) int {
		return strings.Compare(a.Name, b.Name)
	})
	return &Extract{
		AsyncAPIVersion: spec.AsyncAPI,
		Operations:      operations,
	}
}

// messageName picks the most meaningful identifier available for a message.
// Specs in the wild often leave name and title unset on messages that are
// only ever referenced, in which case the referring key is all we have.
func messageName(name, title, fallback string) string {
	if name != "" {
		return name
	}
	if title != "" {
		return title
	}
	return fallback
}
