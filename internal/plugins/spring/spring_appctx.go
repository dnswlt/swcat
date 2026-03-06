package spring

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

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
func ReadApplicationProperties(paths []string) (map[string]string, error) {
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

// FindStreamBindings walks root recursively and scans each file whose full path
// matches any of the given regular expressions. Each matching file is scanned
// individually. It returns a map from file path to its non-empty list of stream bindings.
func FindStreamBindings(root string, patterns []*regexp.Regexp) (map[string][]StreamBinding, error) {
	result := make(map[string][]StreamBinding)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, re := range patterns {
			if re.MatchString(path) {
				props, err := ReadApplicationProperties([]string{path})
				if err != nil {
					return err
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
