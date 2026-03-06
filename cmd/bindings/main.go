// bindings walks a directory tree and extracts Spring Cloud Stream bindings
// from files whose full paths match any of the given regular expressions,
// then outputs the result as JSON.
//
// Usage:
//
//	bindings <root> <pattern> [<pattern>...]
//
// Example:
//
//	bindings /repos '.*/src/main/resources/application.*\.yml'
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/dnswlt/swcat/internal/plugins/spring"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <root> <pattern> [<pattern>...]\n", os.Args[0])
		os.Exit(1)
	}
	root := os.Args[1]
	patterns := make([]*regexp.Regexp, len(os.Args)-2)
	for i, s := range os.Args[2:] {
		re, err := regexp.Compile(s)
		if err != nil {
			log.Fatalf("invalid pattern %q: %v", s, err)
		}
		patterns[i] = re
	}

	result, err := spring.FindStreamBindings(root, patterns)
	if err != nil {
		log.Fatalf("FindStreamBindings: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		log.Fatalf("JSON encode: %v", err)
	}
}
