package v3

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

type Message struct {
	Ref         string     `yaml:"$ref,omitempty"`
	Name        string     `yaml:"name,omitempty"`
	Title       string     `yaml:"title,omitempty"`
	Summary     string     `yaml:"summary,omitempty"`
	Description string     `yaml:"description,omitempty"`
	ContentType string     `yaml:"contentType,omitempty"`
	Payload     *yaml.Node `yaml:"payload,omitempty"`
}

// ReplyAddress is the Reply Address Object: a runtime location (typically a
// message header) that holds the address the reply must be sent to.
type ReplyAddress struct {
	Location    string `yaml:"location,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// OperationReply marks an operation as request/reply. Address and Channel are
// both optional and may both be present: Channel describes the reply channel
// statically, while Address says where the reply is actually routed at runtime.
type OperationReply struct {
	Ref      string        `yaml:"$ref,omitempty"`
	Address  *ReplyAddress `yaml:"address,omitempty"`
	Channel  *Channel      `yaml:"channel,omitempty"`
	Messages []*Message    `yaml:"messages,omitempty"`
}

type Operation struct {
	Ref         string          `yaml:"$ref,omitempty"`
	Action      string          `yaml:"action,omitempty"`
	Channel     *Channel        `yaml:"channel,omitempty"`
	Messages    []*Message      `yaml:"messages,omitempty"`
	Reply       *OperationReply `yaml:"reply,omitempty"`
	Title       string          `yaml:"title,omitempty"`
	Summary     string          `yaml:"summary,omitempty"`
	Description string          `yaml:"description,omitempty"`

	// ChannelName is the name Channel was referenced by, populated by Resolve
	// rather than parsed from the document. It lives on the operation, not on
	// the Channel, because several channel IDs may reference one shared
	// channel object and each must keep its own referring name.
	ChannelName string `yaml:"-"`
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

type Info struct {
	Title   string `yaml:"title"`
	Version string `yaml:"version"`
}

type Spec struct {
	AsyncAPI   string                `yaml:"asyncapi"`
	Info       *Info                 `yaml:"info"`
	Channels   map[string]*Channel   `yaml:"channels"`
	Operations map[string]*Operation `yaml:"operations"`
	Components *Components           `yaml:"components"`
}

func ParseBytes(data []byte) (*Spec, error) {
	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	spec.Resolve()
	return &spec, nil
}

func (s *Spec) Resolve() {
	// Resolve channels first, so that operations referencing them see the
	// resolved addresses and the assigned Name.
	for k, ch := range s.Channels {
		if resolved := s.resolveChannel(ch); resolved != nil {
			s.Channels[k] = resolved
			ch = resolved
		}
		s.resolveChannelMessages(ch)
	}

	for k, op := range s.Operations {
		if op == nil {
			continue
		}
		if op.Ref != "" && s.Components != nil {
			if name, found := strings.CutPrefix(op.Ref, "#/components/operations/"); found {
				if resolved, ok := s.Components.Operations[name]; ok {
					s.Operations[k] = resolved
					op = resolved
				}
			}
		}
		op.ChannelName = channelRefName(op.Channel)
		if resolved := s.resolveChannel(op.Channel); resolved != nil {
			op.Channel = resolved
		}
		s.resolveMessages(op.Messages)

		if op.Reply != nil {
			if resolved := s.resolveChannel(op.Reply.Channel); resolved != nil {
				op.Reply.Channel = resolved
			}
			s.resolveMessages(op.Reply.Messages)
		}
	}

	// Channels reachable only through operations still need their messages
	// resolved; those under s.Channels were handled above.
	if s.Components != nil {
		for _, ch := range s.Components.Channels {
			s.resolveChannelMessages(ch)
		}
	}
}

// parseRef splits a document-local JSON reference such as
// "#/channels/orders/messages/created" into its decoded segments. It returns
// nil for external references, which are not resolved: a spec is read on its
// own, without its neighbours.
//
// A reference is a URI fragment, so each segment is percent-decoded before the
// RFC 6901 escapes for "/" (~1) and "~" (~0) are undone. Decoding per segment
// rather than over the whole fragment keeps a percent-encoded slash inside a
// segment from being mistaken for a separator.
func parseRef(ref string) []string {
	rest, found := strings.CutPrefix(ref, "#/")
	if !found || rest == "" {
		return nil
	}
	segments := strings.Split(rest, "/")
	for i, seg := range segments {
		// An invalid escape sequence is left as written rather than dropped,
		// so the reference still shows up as something recognisable.
		if decoded, err := url.PathUnescape(seg); err == nil {
			seg = decoded
		}
		seg = strings.ReplaceAll(seg, "~1", "/")
		segments[i] = strings.ReplaceAll(seg, "~0", "~")
	}
	return segments
}

// channelRefName reports the name a channel was referenced by. An unresolvable
// reference yields the reference itself, so that it shows up as what it is
// rather than as a blank field.
func channelRefName(ch *Channel) string {
	if ch == nil || ch.Ref == "" {
		return ""
	}
	switch seg := parseRef(ch.Ref); {
	case len(seg) == 2 && seg[0] == "channels":
		return seg[1]
	case len(seg) == 3 && seg[0] == "components" && seg[1] == "channels":
		return seg[2]
	}
	return ch.Ref
}

// resolveChannel follows a channel $ref, supporting both the "#/channels/..."
// form used by operations and the "#/components/channels/..." form. It returns
// nil when there is nothing to resolve, so callers can keep the original.
func (s *Spec) resolveChannel(ch *Channel) *Channel {
	if ch == nil || ch.Ref == "" {
		return nil
	}
	var resolved *Channel
	switch seg := parseRef(ch.Ref); {
	case len(seg) == 2 && seg[0] == "channels":
		resolved = s.Channels[seg[1]]
	case len(seg) == 3 && seg[0] == "components" && seg[1] == "channels":
		if s.Components != nil {
			resolved = s.Components.Channels[seg[2]]
		}
	}
	if resolved == nil || resolved == ch {
		return nil
	}
	return resolved
}

func (s *Spec) resolveChannelMessages(ch *Channel) {
	if ch == nil {
		return
	}
	for mk, msg := range ch.Messages {
		if resolved := s.resolveMessage(msg); resolved != nil {
			ch.Messages[mk] = resolved
		}
	}
}

func (s *Spec) resolveMessages(msgs []*Message) {
	for i, msg := range msgs {
		if resolved := s.resolveMessage(msg); resolved != nil {
			msgs[i] = resolved
		}
	}
}

// resolveMessage follows a message $ref, which may point into components or,
// as an operation's message subset does, into a channel's own messages. The
// referenced key is adopted as the message Name when the message itself
// declares none, so that callers have something to show.
func (s *Spec) resolveMessage(msg *Message) *Message {
	if msg == nil || msg.Ref == "" {
		return nil
	}
	var resolved *Message
	var key string
	switch seg := parseRef(msg.Ref); {
	case len(seg) == 3 && seg[0] == "components" && seg[1] == "messages":
		if s.Components != nil {
			resolved, key = s.Components.Messages[seg[2]], seg[2]
		}
	case len(seg) == 4 && seg[0] == "channels" && seg[2] == "messages":
		if ch := s.Channels[seg[1]]; ch != nil {
			resolved, key = ch.Messages[seg[3]], seg[3]
		}
	}
	if resolved == nil || resolved == msg {
		return nil
	}
	if resolved.Name == "" {
		resolved.Name = key
	}
	return resolved
}
