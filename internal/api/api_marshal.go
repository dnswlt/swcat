package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// Canonical kind names, as used in the YAML "kind:" field and as the
	// in-memory entity kind label (e.g. "kind: Component").
	KindDomain    = "Domain"
	KindSystem    = "System"
	KindComponent = "Component"
	KindResource  = "Resource"
	KindAPI       = "API"
	KindGroup     = "Group"
	// Lowercase kind names, as they appear in the textual form of entity
	// references only (e.g. "resource:ns1/foo"). Stored Ref.Kind values are
	// always canonical; lowercase exists solely at the (de)serialization
	// boundary (ParseRef / Ref.String).
	RefKindDomain    = "domain"
	RefKindSystem    = "system"
	RefKindComponent = "component"
	RefKindResource  = "resource"
	RefKindAPI       = "api"
	RefKindGroup     = "group"
)

var (
	// kindByRefKind maps a lowercase ref kind ("component") to its canonical
	// kind ("Component"); refKindByKind is the inverse.
	kindByRefKind = map[string]string{
		RefKindDomain:    KindDomain,
		RefKindSystem:    KindSystem,
		RefKindComponent: KindComponent,
		RefKindResource:  KindResource,
		RefKindAPI:       KindAPI,
		RefKindGroup:     KindGroup,
	}
	refKindByKind = map[string]string{
		KindDomain:    RefKindDomain,
		KindSystem:    RefKindSystem,
		KindComponent: RefKindComponent,
		KindResource:  RefKindResource,
		KindAPI:       RefKindAPI,
		KindGroup:     RefKindGroup,
	}

	// Regexp defining valid entity names and namespaces
	// Alphanumeric characters and "-". Must start and end with an alphanumeric character.
	validNameRE = regexp.MustCompile("^[A-Za-z]([A-Za-z0-9-]*[A-Za-z0-9])?$")
)

// IsValidKind reports whether kind is a known canonical entity kind ("Component").
func IsValidKind(kind string) bool {
	_, ok := refKindByKind[kind]
	return ok
}

// KindFromRefKind maps a lowercase ref kind ("component") to its canonical kind
// ("Component"). ok reports whether refKind is a valid ref kind.
func KindFromRefKind(refKind string) (kind string, ok bool) {
	kind, ok = kindByRefKind[refKind]
	return
}

// RefKindOf returns the lowercase ref kind for a canonical kind ("Component" ->
// "component"). Unknown kinds (which should not occur for validated refs) degrade
// to a lower-cased rendering so that Ref.String stays usable in error messages.
func RefKindOf(kind string) string {
	if rk, ok := refKindByKind[kind]; ok {
		return rk
	}
	return strings.ToLower(kind)
}

func IsValidName(s string) bool {
	return len(s) > 0 && len(s) <= 63 && validNameRE.MatchString(s)
}
func IsValidNamespace(s string) bool {
	return len(s) > 0 && len(s) <= 63 && validNameRE.MatchString(s)
}

func ParseRef(s string) (*Ref, error) {
	var ref Ref
	// --- Parse the EntityRef part (refStr) ---
	kind, qname, found := strings.Cut(s, ":")
	if found {
		canonical, ok := KindFromRefKind(kind)
		if !ok {
			return nil, fmt.Errorf("invalid entity kind %q", kind)
		}
		ref.Kind = canonical
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

// parseLabelRef parses strings in the format: 'kind:namespace/name [@v1] ["label"]'.
// Both the version @v1 and the label "label" are optional.
func parseLabelRef(s string) (*LabelRef, error) {
	s = strings.TrimSpace(s)
	i := 0
	n := len(s)

	// skipSpace advances the cursor past any whitespace.
	skipSpace := func() {
		for i < n && s[i] == ' ' {
			i++
		}
	}

	// Step 1: Parse the Entity Reference.
	refStart := 0
	for i < n && s[i] != '@' && s[i] != ' ' {
		i++
	}

	ref, err := ParseRef(s[refStart:i])
	if err != nil {
		return nil, err
	}
	labelRef := &LabelRef{
		Ref: ref,
	}
	skipSpace()

	// Step 2: Parse the optional version.
	// A version is identified by a leading '@' and is terminated by whitespace or end of input.
	if i < n && s[i] == '@' {
		i++ // Consume the '@'.
		versionStart := i
		for i < n && s[i] != ' ' {
			i++
		}
		version := s[versionStart:i]
		if version == "" {
			return nil, fmt.Errorf("invalid label ref: empty version in %q", s)
		}
		labelRef.Attrs = map[string]string{
			VersionAttrKey: version,
		}
	}
	skipSpace()

	// Step 3: Parse the optional Label.
	// A label is a double-quoted string.
	if i < n && s[i] == '"' {
		i++ // Consume the opening quote.
		labelStart := i

		// Find the closing quote.
		labelEnd := -1
		for j := i; j < n; j++ {
			if s[j] == '"' {
				labelEnd = j
				break
			}
		}
		if labelEnd == -1 {
			return nil, fmt.Errorf("invalid format: unclosed label in %q", s)
		}
		labelRef.Label = s[labelStart:labelEnd]
		i = labelEnd + 1
	}

	skipSpace()

	// Step 4: Ensure there are no unexpected trailing characters.
	if i < n {
		return nil, fmt.Errorf("invalid format: unexpected trailing characters %q", s[i:])
	}

	return labelRef, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for EntityRef.
// It only supports the simple string format and will return an error if a label is present.
func (er *Ref) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("entity ref must be a string scalar, but got %s", value.Tag)
	}

	ref, err := ParseRef(value.Value)
	if err != nil {
		return err
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
		labelRef, err := parseLabelRef(value.Value)
		if err != nil {
			return err
		}
		*lr = *labelRef
		return nil

	// Case 2: The value is a map, e.g., { ref: "...", label: "..." }
	case yaml.MappingNode:
		var aux struct {
			Ref   string            `yaml:"ref"`
			Label string            `yaml:"label"`
			Attrs map[string]string `yaml:"attrs"`
		}
		// Bit clumsy: to decode this strictly, we have to parse the value again:
		// yaml.Node only has a .Decode() method, but no way to enforce strictness
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		if err := enc.Encode(value); err != nil {
			return fmt.Errorf("failed to re-encode node: %v", err)
		}
		strictDec := yaml.NewDecoder(&buf)
		strictDec.KnownFields(true)
		if err := strictDec.Decode(&aux); err != nil {
			return err
		}

		ref, err := ParseRef(aux.Ref)
		if err != nil {
			return err
		}

		lr.Ref = ref
		lr.Label = aux.Label
		lr.Attrs = aux.Attrs
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into LabelRef", value.Tag)
}

var (
	// Regex to deconstruct v1.2.3, 1.2.3, 1.2., 1, version strings.
	// Groups:
	// 1: Major (\d+)
	// 2: Minor (\d+) - optional
	// 3: Patch (\d+) - optional
	// 4: Suffix (.*) - optional
	versionRegex = regexp.MustCompile(`^(?:v)?(\d+)(?:\.(\d+))?(?:\.(\d+))?(.*)?$`)
)

// UnmarshalYAML implements the gopkg.in/yaml.v3.Unmarshaler interface.
func (v *Version) UnmarshalYAML(node *yaml.Node) error {
	// Must be a plain string.
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("version field must be a plain string, but got %v node", node.Tag)
	}

	// The whole input is the "raw" version.
	v.RawVersion = node.Value

	// Try to parse the string.
	matches := versionRegex.FindStringSubmatch(node.Value)
	if matches == nil {
		// Input didn't match a known version format.
		// Leave the other fields as their zero value and return success.
		return nil
	}

	// We have a match. Populate the struct.
	//    matches[0] is the full string (e.g., "v1.2.3-funky")
	//    matches[1] is Major (e.g., "1")
	//    matches[2] is Minor (e.g., "2" or "")
	//    matches[3] is Patch (e.g., "3" or "")
	//    matches[4] is Suffix (e.g., "-funky" or "")
	parseInt := func(s string) int {
		if s == "" {
			return 0
		}
		// We can ignore the error because the regex `(\d+)` guarantees
		// it's a valid non-negative integer.
		i, _ := strconv.Atoi(s)
		return i
	}
	v.Major = parseInt(matches[1])
	v.Minor = parseInt(matches[2])
	v.Patch = parseInt(matches[3])
	v.Suffix = matches[4] // Suffix is captured with its separator

	return nil
}

// MustMarshalJSON marshals item as (unindented) JSON.
// It must only be called on internal structs that are known to marshal properly.
// It panics on any error.
func MustMarshalJSON(item any) []byte {
	bs, err := json.Marshal(item)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal item of type %T: %v", item, err))
	}
	return bs
}
