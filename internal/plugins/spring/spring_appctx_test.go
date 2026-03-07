package spring

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestReadApplicationProperties(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)

	props, err := ReadApplicationProperties([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ReadApplicationProperties: %v", err)
	}

	keys := make([]string, 0, len(props))
	for k := range props {
		if !strings.HasPrefix(k, "spring.cloud.stream.bindings.") || !strings.HasSuffix(k, ".destination") {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s = %s\n", k, props[k])
	}
}

func TestReadApplicationPropertiesWithImports(t *testing.T) {
	// Create a temporary directory structure for testing:
	// root/
	//   app/
	//     application.yml
	//   config/
	//     application-imported.yml

	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "app")
	configDir := filepath.Join(tempDir, "config")

	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	appYml := filepath.Join(appDir, "application.yml")
	importedYml := filepath.Join(configDir, "application-imported.yml")

	appContent := []byte(`
spring:
  config:
    import: "classpath:application-imported.yml"
  cloud:
    stream:
      bindings:
        myBinding-in-0:
          destination: ${PREFIX}_${topic.name:default_topic}_${application.topics.update-inbound-queuename}
`)
	if err := os.WriteFile(appYml, appContent, 0644); err != nil {
		t.Fatal(err)
	}

	importedContent := []byte(`
PREFIX: ${ENV_VAR}
topic:
  name: my_resolved_topic
  other: ${missing.property}
application:
  topics:
    updateInboundQueuename: my_resolved_relaxed_queue
`)
	if err := os.WriteFile(importedYml, importedContent, 0644); err != nil {
		t.Fatal(err)
	}

	fileIndex := map[string]string{
		"application.yml":          appYml,
		"application-imported.yml": importedYml,
	}

	props, err := ReadApplicationProperties([]string{appYml}, fileIndex)
	if err != nil {
		t.Fatalf("ReadApplicationProperties: %v", err)
	}

	dest := props["spring.cloud.stream.bindings.myBinding-in-0.destination"]

	// Expectations:
	// ${PREFIX} -> ${ENV_VAR} (since ENV_VAR is unresolved)
	// ${topic.name:default_topic} -> my_resolved_topic
	// ${application.topics.update-inbound-queuename} -> my_resolved_relaxed_queue (via relaxed binding)
	// Result: ${ENV_VAR}_my_resolved_topic_my_resolved_relaxed_queue
	expected := "${ENV_VAR}_my_resolved_topic_my_resolved_relaxed_queue"

	if dest != expected {
		t.Errorf("Expected destination %q, got %q", expected, dest)
	}
}

func TestMatchTopics(t *testing.T) {
	tests := []struct {
		name     string
		consumer string
		producer string
		want     bool
	}{
		{
			name:     "Exact match",
			consumer: "a/b/c",
			producer: "a/b/c",
			want:     true,
		},
		{
			name:     "Mismatch",
			consumer: "a/b/c",
			producer: "a/x/c",
			want:     false,
		},
		{
			name:     "Consumer single wildcard",
			consumer: "a/*/c",
			producer: "a/b/c",
			want:     true,
		},
		{
			name:     "Producer single wildcard (e.g. from unresolved prop)",
			consumer: "a/b/c",
			producer: "a/*/c",
			want:     true,
		},
		{
			name:     "Consumer multi-level wildcard",
			consumer: "a/>",
			producer: "a/b/c/d",
			want:     true,
		},
		{
			name:     "Consumer multi-level wildcard exact match",
			consumer: "a/b/c/>",
			producer: "a/b/c",
			want:     true,
		},
		{
			name:     "Consumer unresolved property as wildcard",
			consumer: "a/${missing}/c",
			producer: "a/b/c",
			want:     true,
		},
		{
			name:     "Producer unresolved property as wildcard",
			consumer: "a/b/c",
			producer: "a/${missing}/c",
			want:     true,
		},
		{
			name:     "Multiple unresolved properties",
			consumer: "a/${c_missing}/${d_missing}",
			producer: "a/b/c",
			want:     true,
		},
		{
			name:     "Unresolved property prefixing wildcard",
			consumer: "${missing}/>",
			producer: "foo/bar/baz",
			want:     true,
		},
		{
			name:     "Consumer multi-level wildcard fail",
			consumer: "a/b/>",
			producer: "x/y/z",
			want:     false,
		},
		{
			name:     "Different lengths without wildcards",
			consumer: "a/b",
			producer: "a/b/c",
			want:     false,
		},
		{
			name:     "Multiple asterisks in one level",
			consumer: "a/*middle*/c",
			producer: "a/prefix_middle_suffix/c",
			want:     true,
		},
		{
			name:     "Both consumer and producer have unresolved properties",
			consumer: "a/${CONSUMER_MISSING}/c",
			producer: "a/${PRODUCER_MISSING}/c",
			want:     true,
		},
		{
			name:     "Both unresolved properties as suffixes",
			consumer: "a/prefix_${CONSUMER_MISSING}/c",
			producer: "a/prefix_${PRODUCER_MISSING}/c",
			want:     true, // Treated as `prefix_*` matching `prefix_*`, which trivially yields true under path.Match
		},
		{
			name:     "Unresolved properties with mismatches",
			consumer: "a/foo_${CONSUMER_MISSING}/c",
			producer: "a/bar_${PRODUCER_MISSING}/c",
			want:     false, // `foo_*` does not match `bar_*`
		},
		{
			name:     "Ignore consumer replyTopicWithWildcards",
			consumer: "${replyTopicWithWildcards|requestTagesFahrplanAbleitung|*}",
			producer: "some/producer/topic",
			want:     false,
		},
		{
			name:     "Ignore producer replyTopicWithWildcards",
			consumer: "some/consumer/topic",
			producer: "a/b/${replyTopicWithWildcards|uuid}",
			want:     false,
		},
		{
			name:     "Ignore both replyTopicWithWildcards",
			consumer: "${replyTopicWithWildcards|requestTaxi|*}",
			producer: "a/b/${replyTopicWithWildcards|uuid}",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchTopics(tt.consumer, tt.producer); got != tt.want {
				t.Errorf("MatchTopics(%q, %q) = %v, want %v", tt.consumer, tt.producer, got, tt.want)
			}
		})
	}
}
