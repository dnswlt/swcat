package asyncapi

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseV3Operations(t *testing.T) {
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
operations:
  op1:
    action: receive
    channel:
      $ref: '#/channels/ch1'
components:
  messages:
    msg1:
      name: message1
`
	extract, err := ParseBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	if extract.AsyncAPIVersion != "3.0.0" {
		t.Errorf("AsyncAPIVersion = %q, want %q", extract.AsyncAPIVersion, "3.0.0")
	}
	// v3 specs are reported as operations, never as channels.
	if extract.Channels != nil {
		t.Errorf("Channels = %+v, want nil for a v3 spec", extract.Channels)
	}
	if len(extract.Operations) != 1 {
		t.Fatalf("got %d operations, want 1", len(extract.Operations))
	}

	op := extract.Operations[0]
	if op.Name != "op1" {
		t.Errorf("Name = %q, want %q", op.Name, "op1")
	}
	if op.Action != "receive" {
		t.Errorf("Action = %q, want %q", op.Action, "receive")
	}
	if op.Channel != "ch1" {
		t.Errorf("Channel = %q, want %q", op.Channel, "ch1")
	}
	if op.Address != "addr1" {
		t.Errorf("Address = %q, want %q", op.Address, "addr1")
	}
	if op.Reply {
		t.Error("Reply = true, want false for an operation without a reply")
	}
	// The operation declares no message subset, so it inherits the channel's.
	// msg1 resolves through components and reports its declared name.
	want := []string{"message1", "message2"}
	if !slices.Equal(op.Messages, want) {
		t.Errorf("Messages = %v, want %v", op.Messages, want)
	}
}

// TestParseV3RequestReply covers the shape produced by Solace-style
// request/reply specs: operations reference channels via "#/channels/...",
// replies name both a static channel and a dynamic runtime address, and the
// messages themselves declare no name of their own.
func TestParseV3RequestReply(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
info:
  title: Flights Reference Data
  version: 4.2.1
channels:
  AirportRequest:
    address: flights/avail/v1/ref/Airport/request
    messages:
      requestAirportMessage:
        $ref: '#/components/messages/AirportRequest'
  AirportReply:
    address: flights/avail/v1/ref/Airport/reply
    messages:
      responseMessage:
        $ref: '#/components/messages/AirportResponse'
  ScheduleChangedChannel:
    address: flights/avail/v1/ref/notification/update
    messages:
      updateMessage:
        $ref: '#/components/messages/ScheduleChangedMsg'
operations:
  AirportRequest:
    action: receive
    channel:
      $ref: '#/channels/AirportRequest'
    reply:
      address:
        location: "$message.header#/replyChannel"
      channel:
        $ref: '#/channels/AirportReply'
  BaggageAllowanceRequest:
    action: receive
    channel:
      $ref: '#/channels/AirportRequest'
    reply:
      address:
        location: "$message.header#/replyChannel"
  ScheduleChangedMsg:
    action: send
    channel:
      $ref: '#/channels/ScheduleChangedChannel'
components:
  messages:
    AirportRequest:
      description: Request Message to get AirportMsg elements
    AirportResponse:
      description: Response message to the AirportRequest
    ScheduleChangedMsg:
      description: A notification that newer reference-data is available.
`
	extract, err := ParseBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	// Operations are sorted by name; the reply channel is not itself an
	// operation and so is not reported.
	var names []string
	for _, op := range extract.Operations {
		names = append(names, op.Name)
	}
	wantNames := []string{"AirportRequest", "BaggageAllowanceRequest", "ScheduleChangedMsg"}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("operation names = %v, want %v", names, wantNames)
	}

	byName := map[string]*SimpleOperation{}
	for _, op := range extract.Operations {
		byName[op.Name] = op
	}

	// A reply naming a static channel.
	if op := byName["AirportRequest"]; !op.Reply {
		t.Error("AirportRequest: Reply = false, want true")
	} else if op.Address != "flights/avail/v1/ref/Airport/request" {
		t.Errorf("AirportRequest: Address = %q, want the request channel address", op.Address)
	} else if !slices.Equal(op.Messages, []string{"AirportRequest"}) {
		// The message declares no name, so the component key stands in for it.
		t.Errorf("AirportRequest: Messages = %v, want [AirportRequest]", op.Messages)
	}

	// A reply routed purely dynamically, with no reply channel declared.
	if op := byName["BaggageAllowanceRequest"]; !op.Reply {
		t.Error("BaggageAllowanceRequest: Reply = false, want true")
	}

	// A fire-and-forget notification.
	if op := byName["ScheduleChangedMsg"]; op.Reply {
		t.Error("ScheduleChangedMsg: Reply = true, want false")
	} else if op.Action != "send" {
		t.Errorf("ScheduleChangedMsg: Action = %q, want %q", op.Action, "send")
	}
}

func TestParseV2Channels(t *testing.T) {
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
	// Exercise the file-reading path too.
	tmpFile := filepath.Join(t.TempDir(), "asyncapi_v2.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	extract, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if extract.AsyncAPIVersion != "2.6.0" {
		t.Errorf("AsyncAPIVersion = %q, want %q", extract.AsyncAPIVersion, "2.6.0")
	}
	// v2 specs are reported as channels, never as operations.
	if extract.Operations != nil {
		t.Errorf("Operations = %+v, want nil for a v2 spec", extract.Operations)
	}
	if len(extract.Channels) != 1 {
		t.Fatalf("got %d channels, want 1", len(extract.Channels))
	}

	ch := extract.Channels[0]
	// In v2, Name and Address are the same (the map key).
	if ch.Name != "user/signedup" || ch.Address != "user/signedup" {
		t.Errorf("Name/Address = %q/%q, want %q for both", ch.Name, ch.Address, "user/signedup")
	}

	slices.Sort(ch.Messages)
	if got := strings.Join(ch.Messages, ","); got != "UserSignedUp,inlineMsg" {
		t.Errorf("Messages = %q, want %q", got, "UserSignedUp,inlineMsg")
	}
}

func TestParseUnsupportedVersion(t *testing.T) {
	if _, err := ParseBytes([]byte("asyncapi: '1.2.0'\n")); err == nil {
		t.Fatal("ParseBytes() error = nil, want an unsupported-version error")
	}
}

// TestParseSlightlyInvalidSpecsDoesNotPanic covers malformed-but-plausible
// documents produced while editing a spec by hand. Whether these entries are
// skipped, preserved, or rejected is deliberately left to the parser; none of
// them should be able to panic the importer.
func TestParseSlightlyInvalidSpecsDoesNotPanic(t *testing.T) {
	tests := []struct {
		name string
		spec string
	}{
		{
			name: "v2 null channel",
			spec: `
asyncapi: '2.6.0'
channels:
  broken: null
components:
  messages: {}
`,
		},
		{
			name: "v2 reference to null component message",
			spec: `
asyncapi: '2.6.0'
channels:
  jobs:
    publish:
      message:
        $ref: '#/components/messages/MissingBody'
components:
  messages:
    MissingBody: null
`,
		},
		{
			name: "v3 null operation message",
			spec: `
asyncapi: '3.0.0'
channels:
  jobs:
    address: jobs
operations:
  sendJob:
    action: send
    channel:
      $ref: '#/channels/jobs'
    messages:
      - null
`,
		},
		{
			name: "v3 null inherited channel message",
			spec: `
asyncapi: '3.0.0'
channels:
  jobs:
    address: jobs
    messages:
      broken: null
operations:
  sendJob:
    action: send
    channel:
      $ref: '#/channels/jobs'
`,
		},
		{
			name: "v3 reference to null component operation",
			spec: `
asyncapi: '3.0.0'
operations:
  sendJob:
    $ref: '#/components/operations/NullOperation'
components:
  operations:
    NullOperation: null
`,
		},
		{
			name: "v2 null publish operation",
			spec: `
asyncapi: '2.6.0'
channels:
  jobs:
    publish: null
    subscribe:
      message: null
`,
		},
		{
			name: "v2 null components section",
			spec: `
asyncapi: '2.6.0'
channels:
  jobs:
    publish:
      message:
        $ref: '#/components/messages/Gone'
components:
  messages: null
`,
		},
		{
			name: "v3 null reply parts",
			spec: `
asyncapi: '3.0.0'
operations:
  sendJob:
    action: send
    reply:
      address: null
      channel: null
      messages:
        - null
`,
		},
		{
			name: "v3 reference to null component channel",
			spec: `
asyncapi: '3.0.0'
channels:
  alias:
    $ref: '#/components/channels/Missing'
operations:
  sendJob:
    action: send
    channel:
      $ref: '#/channels/alias'
components:
  channels:
    Missing: null
`,
		},
		{
			name: "v3 reference to null channel-scoped message",
			spec: `
asyncapi: '3.0.0'
channels:
  jobs:
    address: jobs
    messages:
      broken: null
operations:
  sendJob:
    action: send
    channel:
      $ref: '#/channels/jobs'
    messages:
      - $ref: '#/channels/jobs/messages/broken'
`,
		},
		{
			name: "v3 every section null",
			spec: `
asyncapi: '3.0.0'
channels: null
operations: null
components: null
`,
		},
		{
			name: "v3 self-referential channel",
			spec: `
asyncapi: '3.0.0'
channels:
  loop:
    $ref: '#/channels/loop'
operations:
  sendJob:
    action: send
    channel:
      $ref: '#/channels/loop'
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("ParseBytes() panicked: %v", recovered)
				}
			}()

			_, _ = ParseBytes([]byte(tt.spec))
		})
	}
}

// TestParseV3MessageSubsets covers operation.messages, which references the
// channel's own messages rather than components, and distinguishes an omitted
// list ("all channel messages") from an explicitly empty one ("none").
func TestParseV3MessageSubsets(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
channels:
  orders:
    address: orders/v1
    messages:
      created:
        name: OrderCreated
      cancelled:
        $ref: '#/components/messages/OrderCancelled'
      unnamed: {}
operations:
  subset:
    action: send
    channel:
      $ref: '#/channels/orders'
    messages:
      - $ref: '#/channels/orders/messages/created'
      - $ref: '#/channels/orders/messages/cancelled'
  omitted:
    action: send
    channel:
      $ref: '#/channels/orders'
  explicitlyEmpty:
    action: send
    channel:
      $ref: '#/channels/orders'
    messages: []
components:
  messages:
    OrderCancelled: {}
`
	extract, err := ParseBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	byName := map[string][]string{}
	for _, op := range extract.Operations {
		byName[op.Name] = op.Messages
	}

	// Channel-scoped refs resolve, and a message with no name of its own
	// falls back to the key it is referenced by.
	if want := []string{"OrderCancelled", "OrderCreated"}; !slices.Equal(byName["subset"], want) {
		t.Errorf("subset messages = %v, want %v", byName["subset"], want)
	}
	// Omitted: every message on the channel, including the unnamed one.
	if want := []string{"OrderCancelled", "OrderCreated", "unnamed"}; !slices.Equal(byName["omitted"], want) {
		t.Errorf("omitted messages = %v, want %v", byName["omitted"], want)
	}
	// Explicitly empty: no messages, rather than all of them.
	if got := byName["explicitlyEmpty"]; len(got) != 0 {
		t.Errorf("explicitlyEmpty messages = %v, want none", got)
	}
}

// TestParseV3AliasedChannels checks that two channel IDs referencing one
// shared component channel each keep their own name. The referring name must
// not be stored on the shared channel, where map iteration order would decide
// which alias wins.
func TestParseV3AliasedChannels(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
channels:
  aliasA:
    $ref: '#/components/channels/shared'
  aliasB:
    $ref: '#/components/channels/shared'
operations:
  opA:
    action: send
    channel:
      $ref: '#/channels/aliasA'
  opB:
    action: send
    channel:
      $ref: '#/channels/aliasB'
components:
  channels:
    shared:
      address: shared/addr
`
	// Run repeatedly: the failure this guards against is decided by Go's
	// randomized map iteration order.
	for i := 0; i < 20; i++ {
		extract, err := ParseBytes([]byte(yamlContent))
		if err != nil {
			t.Fatalf("ParseBytes() error = %v", err)
		}
		got := map[string]string{}
		for _, op := range extract.Operations {
			got[op.Name] = op.Channel
			if op.Address != "shared/addr" {
				t.Fatalf("%s: Address = %q, want the shared channel address", op.Name, op.Address)
			}
		}
		if got["opA"] != "aliasA" || got["opB"] != "aliasB" {
			t.Fatalf("channel names = %v, want opA→aliasA and opB→aliasB", got)
		}
	}
}

// TestParseV3UnresolvableRefs checks that references we cannot follow — an
// external document, or a JSON Pointer into something we do not index — are
// reported as the reference itself rather than as a blank field.
func TestParseV3UnresolvableRefs(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
channels:
  local:
    address: local/addr
operations:
  externalChannel:
    action: send
    channel:
      $ref: 'other.yaml#/channels/remote'
    messages:
      - $ref: 'other.yaml#/components/messages/Remote'
  externalOperation:
    $ref: 'other.yaml#/operations/remoteOp'
`
	extract, err := ParseBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	byName := map[string]*SimpleOperation{}
	for _, op := range extract.Operations {
		byName[op.Name] = op
	}

	// An unresolved channel reference stands in for the address we lack.
	op := byName["externalChannel"]
	if op.Channel != "other.yaml#/channels/remote" {
		t.Errorf("Channel = %q, want the unresolved reference", op.Channel)
	}
	if want := []string{"other.yaml#/components/messages/Remote"}; !slices.Equal(op.Messages, want) {
		t.Errorf("Messages = %v, want %v", op.Messages, want)
	}

	// An operation that is itself an unresolved $ref keeps that reference and
	// asserts nothing about how it communicates.
	op = byName["externalOperation"]
	if op.Ref != "other.yaml#/operations/remoteOp" {
		t.Errorf("Ref = %q, want the unresolved reference", op.Ref)
	}
	if op.Action != "" || op.Channel != "" || op.Address != "" {
		t.Errorf("unresolved operation should assert nothing, got %+v", *op)
	}
	if op.Reply {
		t.Error("Reply = true, want false on an operation we know nothing about")
	}

	// A resolvable operation $ref must not leave Ref behind.
	resolved, err := ParseBytes([]byte(`
asyncapi: '3.0.0'
channels:
  ch: {address: a}
operations:
  local:
    $ref: '#/components/operations/realOp'
components:
  operations:
    realOp:
      action: send
      channel: {$ref: '#/channels/ch'}
`))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if got := resolved.Operations[0]; got.Ref != "" || got.Action != "send" {
		t.Errorf("resolved operation = %+v, want Ref empty and Action send", *got)
	}
}

// TestParseV3PercentEncodedRefs covers references that carry percent-encoding.
// A reference is a URI fragment, so it must be percent-decoded before the
// RFC 6901 escapes are undone.
func TestParseV3PercentEncodedRefs(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
channels:
  sales ops:
    address: sales/ops/addr
    messages:
      created:
        name: Created
  tilde~slash:
    address: tilde/addr
operations:
  spaced:
    action: send
    channel:
      $ref: '#/channels/sales%20ops'
    messages:
      - $ref: '#/channels/sales%20ops/messages/created'
  tilded:
    action: send
    channel:
      $ref: '#/channels/tilde%7E0slash'
`
	extract, err := ParseBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	byName := map[string]*SimpleOperation{}
	for _, op := range extract.Operations {
		byName[op.Name] = op
	}

	if op := byName["spaced"]; op.Channel != "sales ops" || op.Address != "sales/ops/addr" {
		t.Errorf("spaced: Channel/Address = %q/%q, want %q/%q",
			op.Channel, op.Address, "sales ops", "sales/ops/addr")
	} else if want := []string{"Created"}; !slices.Equal(op.Messages, want) {
		t.Errorf("spaced: Messages = %v, want %v", op.Messages, want)
	}

	// %7E decodes to "~", which then combines with the following "0" into the
	// RFC 6901 escape for a literal tilde.
	if op := byName["tilded"]; op.Channel != "tilde~slash" || op.Address != "tilde/addr" {
		t.Errorf("tilded: Channel/Address = %q/%q, want %q/%q",
			op.Channel, op.Address, "tilde~slash", "tilde/addr")
	}
}

// TestParseV3EscapedRefs checks RFC 6901 escaping, which a channel ID
// containing a slash relies on.
func TestParseV3EscapedRefs(t *testing.T) {
	yamlContent := `
asyncapi: '3.0.0'
channels:
  foo/bar:
    address: foo/bar/addr
    messages:
      created:
        name: Created
operations:
  op:
    action: send
    channel:
      $ref: '#/channels/foo~1bar'
    messages:
      - $ref: '#/channels/foo~1bar/messages/created'
`
	extract, err := ParseBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	op := extract.Operations[0]
	if op.Channel != "foo/bar" || op.Address != "foo/bar/addr" {
		t.Errorf("Channel/Address = %q/%q, want %q/%q", op.Channel, op.Address, "foo/bar", "foo/bar/addr")
	}
	if want := []string{"Created"}; !slices.Equal(op.Messages, want) {
		t.Errorf("Messages = %v, want %v", op.Messages, want)
	}
}
