package catalog

import (
	"regexp"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
)

var (
	// qualifiedNameRegex validates the name segment of a key and non-empty values.
	// It must start and end with an alphanumeric character, with dashes, underscores,
	// dots, and alphanumerics allowed in between.
	qualifiedNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([-a-zA-Z0-9_.]*[a-zA-Z0-9])?$`)

	// dnsLabelRegex validates each segment of a DNS subdomain (the optional key prefix).
	// It must start and end with a lowercase alphanumeric character, with dashes
	// allowed in between.
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// tagRegex validates the tag format. It must consist of one or more
	// segments of [a-z0-9:+#], separated by single hyphens.
	tagRegex = regexp.MustCompile(`^[a-z0-9:+#]+(-[a-z0-9:+#]+)*$`)
)

func IsValidKind(kind string) bool {
	return api.IsValidRefKind(kind)
}

func IsValidName(s string) bool {
	return api.IsValidName(s)
}
func IsValidNamespace(s string) bool {
	return api.IsValidNamespace(s)
}

// IsValidLabel checks if "key: value" is a valid metadata label.
// Validation follows the rules outlined in
// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
func IsValidLabel(key string, value string) bool {
	return isValidKey(key) && isValidValue(value)
}

// IsValidAnnotation checks if "key: value" is a valid metadata annotation.
// Validation follows the rules outlined in
// https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
func IsValidAnnotation(key string, value string) bool {
	return isValidKey(key)
}

// IsValidTag checks if tag is a valid tag according to
// https://backstage.io/docs/features/software-catalog/descriptor-format/#tags-optional
func IsValidTag(tag string) bool {
	// 1. Check total length constraint.
	if len(tag) > 63 {
		return false
	}

	// 2. Check character and structural constraints.
	return tagRegex.MatchString(tag)
}

// isValidKey validates the label key according to Kubernetes rules.
func isValidKey(key string) bool {
	// A key is composed of an optional prefix and a required name, separated by a slash.
	parts := strings.Split(key, "/")
	var prefix, name string

	switch len(parts) {
	case 1:
		name = parts[0]
	case 2:
		prefix, name = parts[0], parts[1]
		if len(prefix) == 0 {
			// An empty prefix is not allowed if the slash is present.
			return false
		}
	default:
		// More than one slash is invalid.
		return false
	}

	// Validate the name segment.
	// It must be between 1 and 63 characters.
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	if !qualifiedNameRegex.MatchString(name) {
		return false
	}

	// If a prefix exists, validate it as a DNS subdomain.
	if len(prefix) > 0 {
		// The total length must not exceed 253 characters.
		if len(prefix) > 253 {
			return false
		}
		// It must consist of one or more DNS labels separated by dots.
		for _, label := range strings.Split(prefix, ".") {
			// Each label must be between 1 and 63 characters.
			if len(label) == 0 || len(label) > 63 {
				return false
			}
			if !dnsLabelRegex.MatchString(label) {
				return false
			}
		}
	}

	return true
}

// isValidValue validates the label value according to Kubernetes rules.
func isValidValue(value string) bool {
	// The value must be 63 characters or less.
	if len(value) > 63 {
		return false
	}

	// An empty value is valid.
	if len(value) == 0 {
		return true
	}

	// If not empty, it must match the same content rules as a key's name segment.
	return qualifiedNameRegex.MatchString(value)
}
