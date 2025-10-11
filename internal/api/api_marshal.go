package api

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// Uppercase kind names, as used in YAML (e.g, "kind: Component")
	YAMLKindDomain    = "Domain"
	YAMLKindSystem    = "System"
	YAMLKindComponent = "Component"
	YAMLKindResource  = "Resource"
	YAMLKindAPI       = "API"
	YAMLKindGroup     = "Group"
	// Lowercase kind names, as used in entity references (e.g. "resource:ns1/foo")
	KindDomain    = "domain"
	KindSystem    = "system"
	KindComponent = "component"
	KindResource  = "resource"
	KindAPI       = "api"
	KindGroup     = "group"
)

var (
	// Valid entity kinds for use in entity references
	validRefKinds = map[string]bool{
		KindDomain:    true,
		KindSystem:    true,
		KindComponent: true,
		KindResource:  true,
		KindAPI:       true,
		KindGroup:     true,
	}

	// Regexp defining valid entity names and namespaces
	validNameRE = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_-]*$")
)

func IsValidRefKind(kind string) bool {
	_, ok := validRefKinds[kind]
	return ok
}

func IsValidName(s string) bool {
	return validNameRE.MatchString(s)
}
func IsValidNamespace(s string) bool {
	return validNameRE.MatchString(s)
}

func ParseRef(s string) (*Ref, error) {
	var ref Ref
	// --- Parse the EntityRef part (refStr) ---
	kind, qname, found := strings.Cut(s, ":")
	if found {
		if !IsValidRefKind(kind) {
			return nil, fmt.Errorf("invalid entity kind %q", kind)
		}
		ref.Kind = kind
	} else {
		// No kind: specified
		qname = s
	}

	ns, name, found := strings.Cut(qname, "/")
	if found {
		if !IsValidNamespace(ns) {
			return nil, fmt.Errorf("invalid namespace %q", ns)
		}
		if !IsValidName(name) {
			return nil, fmt.Errorf("invalid name %q", name)
		}
		ref.Namespace = ns
		ref.Name = name
	} else {
		if !IsValidName(qname) {
			return nil, fmt.Errorf("invalid name %q", qname)
		}
		ref.Namespace = DefaultNamespace
		ref.Name = qname
	}
	return &ref, nil
}

// parseLabelRefString parses a string into an EntityRef and an optional label.
// It parses strings in the format: 'kind:namespace/name "Optional Label"'
func parseLabelRefString(s string) (ref *Ref, label string, err error) {
	// Split the string by the first space to separate the ref from the optional label.
	refStr, labelStr, _ := strings.Cut(s, " ")
	ref, err = ParseRef(refStr)
	if err != nil {
		return
	}
	// Parse the Label part, which must be enclosed in double quotes.
	label = strings.TrimSpace(labelStr)
	if label != "" {
		if len(label) > 1 && strings.HasPrefix(label, `"`) && strings.HasSuffix(label, `"`) {
			label = label[1 : len(label)-1]
		} else {
			err = fmt.Errorf("label must be enclosed in double quotes, got '%s'", label)
			return
		}
	}
	return
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for EntityRef.
// It only supports the simple string format and will return an error if a label is present.
func (er *Ref) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("entity ref must be a string scalar, but got %s", value.Tag)
	}

	ref, label, err := parseLabelRefString(value.Value)
	if err != nil {
		return err
	}

	if label != "" {
		return fmt.Errorf("entity ref '%s' cannot have a label", value.Value)
	}

	*er = *ref
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for LabelRef.
// It supports both the simple string format (with or without a quoted label)
// and the record-style map format.
func (lr *LabelRef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	// Case 1: The value is a simple string, e.g., "kind:ns/name "label""
	case yaml.ScalarNode:
		ref, label, err := parseLabelRefString(value.Value)
		if err != nil {
			return err
		}
		lr.Ref = ref
		lr.Label = label
		return nil

	// Case 2: The value is a map, e.g., { ref: "...", label: "..." }
	case yaml.MappingNode:
		var aux struct {
			Ref   string `yaml:"ref"`
			Label string `yaml:"label"`
		}
		if err := value.Decode(&aux); err != nil {
			return err
		}

		// The 'ref' field within the map must not contain its own label.
		ref, label, err := parseLabelRefString(aux.Ref)
		if err != nil {
			return err
		}
		if label != "" {
			return fmt.Errorf("ref field '%s' inside a record cannot contain its own label", aux.Ref)
		}

		lr.Ref = ref
		lr.Label = aux.Label
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into LabelRef", value.Tag)
}
