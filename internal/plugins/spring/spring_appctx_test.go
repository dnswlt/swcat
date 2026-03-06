package spring

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestReadApplicationProperties(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)

	props, err := ReadApplicationProperties([]string{dir})
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

	t.Fail()
}
