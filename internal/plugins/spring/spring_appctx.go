package spring

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var applicationYMLPattern = regexp.MustCompile(`^application(-[^.]+)?\.yml$`)

var bindingDestinationKey = regexp.MustCompile(`^spring\.cloud\.stream\.bindings\.(.+)\.destination$`)
var bindingNamePattern = regexp.MustCompile(`^(.+)-(in|out)-\d+$`)

// BindingDirection indicates whether a stream binding is an input (consumer) or output (producer).
type BindingDirection string

const (
	BindingIn  BindingDirection = "in"
	BindingOut BindingDirection = "out"
)

// StreamBinding represents a Spring Cloud Stream binding with its destination and binder.
type StreamBinding struct {
	BindingName string           // full name, e.g. "perronLockSender-out-0"
	Direction   BindingDirection // "in" or "out"
	Destination string           // topic/queue name
	Binder      string           // binder name, e.g. "kafka" or "solace"
}

// ReadApplicationProperties takes a list of paths, which can be directories or files,
// and reads all application[-*].yml files in all given directories as well as all given files,
// as Spring application properties YAML files.
//
// It returns a mapping from flattened keys ("my.funny.property") to their values.
func ReadApplicationProperties(paths []string, fileIndex map[string]string) (map[string]string, error) {
	result := make(map[string]string)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("spring properties: cannot stat %s: %w", path, err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, fmt.Errorf("spring properties: cannot read directory %s: %w", path, err)
			}
			for _, entry := range entries {
				if !entry.IsDir() && applicationYMLPattern.MatchString(entry.Name()) {
					if err := readAndMerge(filepath.Join(path, entry.Name()), result); err != nil {
						return nil, err
					}
				}
			}
		} else {
			if err := readAndMerge(path, result); err != nil {
				return nil, err
			}
		}
	}

	if err := processImports(result, fileIndex); err != nil {
		return nil, err
	}

	resolvePlaceholders(result)

	return result, nil
}

func readAndMerge(path string, result map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("spring properties: cannot read %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var root any
		if err := dec.Decode(&root); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("spring properties: cannot parse %s: %w", path, err)
		}
		flattenValue(root, "", result)
	}
	return nil
}

func flattenValue(v any, prefix string, result map[string]string) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenValue(child, key, result)
		}
	case []any:
		for i, child := range val {
			flattenValue(child, fmt.Sprintf("%s[%d]", prefix, i), result)
		}
	case nil:
		// skip null values
	default:
		if _, exists := result[prefix]; !exists {
			result[prefix] = fmt.Sprintf("%v", val)
		}
	}
}

func processImports(result map[string]string, fileIndex map[string]string) error {
	imported := make(map[string]bool)
	importKeyPattern := regexp.MustCompile(`^spring\.config\.import(\[\d+\])?$`)

	for {
		var toImport []string
		for k, v := range result {
			if importKeyPattern.MatchString(k) {
				parts := strings.Split(v, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					p = strings.TrimPrefix(p, "optional:")
					p = strings.TrimPrefix(p, "classpath:")
					p = strings.TrimPrefix(p, "file:")

					if p != "" && !imported[p] {
						toImport = append(toImport, p)
						imported[p] = true
					}
				}
			}
		}

		if len(toImport) == 0 {
			break
		}

		for _, imp := range toImport {
			basename := filepath.Base(imp)
			if path, ok := fileIndex[basename]; ok {
				// error can be ignored heuristically, but let's log it or just silently skip.
				_ = readAndMerge(path, result)
			}
		}
	}
	return nil
}

// normalizeKey converts a Spring property key to a normalized form for relaxed binding.
// It removes dashes and underscores and converts the key to lowercase.
func normalizeKey(key string) string {
	k := strings.ReplaceAll(key, "-", "")
	k = strings.ReplaceAll(k, "_", "")
	return strings.ToLower(k)
}

// lookupRelaxed attempts to find a key in the properties map using Spring's relaxed binding rules.
func lookupRelaxed(props map[string]string, key string) (string, bool) {
	// Fast path: exact match
	if val, ok := props[key]; ok {
		return val, true
	}

	// Slow path: normalized match
	normKey := normalizeKey(key)
	// For better performance on repeated lookups, a normalized index could be built,
	// but mapping sizes are typically small enough that a linear scan is fine.
	for k, v := range props {
		if normalizeKey(k) == normKey {
			return v, true
		}
	}

	return "", false
}

// resolvePlaceholders iteratively resolves Spring property placeholders in the format ${key} or ${key:default}.
// It performs up to 10 passes to handle nested placeholders (e.g. ${prefix_${suffix}}).
func resolvePlaceholders(result map[string]string) {
	placeholderRe := regexp.MustCompile(`\${([^}]+)}`)
	changed := true
	iterations := 0

	// Limit iterations to prevent infinite loops in cases of circular references
	for changed && iterations < 10 {
		changed = false
		for k, v := range result {
			newV := placeholderRe.ReplaceAllStringFunc(v, func(match string) string {
				// match is e.g. "${my.property:default}" -> key becomes "my.property:default"
				key := match[2 : len(match)-1]

				var defaultVal string
				hasDefault := false

				// Handle default values separated by ':'
				if idx := strings.IndexByte(key, ':'); idx != -1 {
					defaultVal = key[idx+1:]
					key = key[:idx]
					hasDefault = true
				}

				// Attempt to resolve the key, using relaxed binding to handle mismatching cases/dashes
				if resolved, ok := lookupRelaxed(result, key); ok {
					return resolved
				}

				// If not found in properties, use default if provided
				if hasDefault {
					return defaultVal
				}

				// If no match and no default, leave the placeholder unchanged (might be an ENV var)
				return match
			})
			if newV != v {
				result[k] = newV
				changed = true
			}
		}
		iterations++
	}
}

// FindStreamBindings walks root recursively and scans each file whose full path
// matches any of the given regular expressions. Each matching file is scanned
// individually. It returns a map from file path to its non-empty list of stream bindings.
func FindStreamBindings(root string, patterns []*regexp.Regexp) (map[string][]StreamBinding, error) {
	fileIndex := make(map[string]string)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext == ".yml" || ext == ".yaml" {
			fileIndex[filepath.Base(path)] = path
		}
		return nil
	})

	result := make(map[string][]StreamBinding)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, re := range patterns {
			if re.MatchString(path) {
				props, err := ReadApplicationProperties([]string{path}, fileIndex)
				if err != nil {
					log.Printf("Could not read %s: %v", path, err)
					break
				}
				if bindings := StreamBindings(props); len(bindings) > 0 {
					result[path] = bindings
				}
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// StreamBindings extracts all Spring Cloud Stream bindings from a flattened properties map.
// It returns one StreamBinding per binding that has a destination property.
func StreamBindings(props map[string]string) []StreamBinding {
	var bindings []StreamBinding
	for k, v := range props {
		m := bindingDestinationKey.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		bindingName := m[1]
		b := StreamBinding{
			BindingName: bindingName,
			Destination: v,
		}
		if nm := bindingNamePattern.FindStringSubmatch(bindingName); nm != nil {
			b.Direction = BindingDirection(nm[2])
		}
		if binder, ok := props["spring.cloud.stream.bindings."+bindingName+".binder"]; ok {
			b.Binder = binder
		}
		bindings = append(bindings, b)
	}
	return bindings
}

// MatchTopics compares a consumer topic (which may contain Solace wildcards '*' or '>')
// against a producer topic. Both topics may contain unresolved properties (e.g., "${...}"),
// which are treated equivalently to a single-level '*' wildcard.
// Bindings containing Spring Cloud Stream Request/Reply variables like
// ${replyTopicWithWildcards|...} are ignored and will never match.
func MatchTopics(consumerTopic, producerTopic string) bool {
	if strings.Contains(consumerTopic, "${replyTopicWithWildcards|") ||
		strings.Contains(producerTopic, "${replyTopicWithWildcards|") {
		return false
	}

	// Treat unresolved properties as '*' logically.
	c := normalizePlaceholders(consumerTopic)
	p := normalizePlaceholders(producerTopic)

	cLevels := strings.Split(c, "/")
	pLevels := strings.Split(p, "/")

	return matchLevels(cLevels, pLevels)
}

func normalizePlaceholders(topic string) string {
	placeholderRe := regexp.MustCompile(`\${[^}]+}`)
	return placeholderRe.ReplaceAllString(topic, "*")
}

func matchLevels(cLevels, pLevels []string) bool {
	i, j := 0, 0
	for i < len(cLevels) && j < len(pLevels) {
		if cLevels[i] == ">" {
			// '>' matches remaining levels, but must be the last token in consumer topic
			return i == len(cLevels)-1
		}

		cMatch, _ := path.Match(cLevels[i], pLevels[j])
		pMatch, _ := path.Match(pLevels[j], cLevels[i])
		if !cMatch && !pMatch {
			return false
		}
		i++
		j++
	}

	// If we've exhausted consumer levels, producer levels must also be exhausted.
	if i == len(cLevels) {
		return j == len(pLevels)
	}

	// If we've exhausted producer levels but not consumer, check if the only remaining
	// consumer level is '>' (which can match an empty sequence of remaining levels).
	if j == len(pLevels) && i == len(cLevels)-1 && cLevels[i] == ">" {
		return true
	}

	return false
}
