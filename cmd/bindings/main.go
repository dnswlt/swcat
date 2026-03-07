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

	// build dependency graph
	// A depends on B if A has an "in" binding that matches an "out" binding of B
	graph := make(map[string][]string)

	for consumerApp, consumerBindings := range result {
		// use a map to deduplicate dependencies
		deps := make(map[string]bool)

		for _, consumB := range consumerBindings {
			if consumB.Direction != spring.BindingIn {
				continue
			}

			for producerApp, producerBindings := range result {
				if consumerApp == producerApp {
					continue
				}

				for _, prodB := range producerBindings {
					if prodB.Direction != spring.BindingOut {
						continue
					}

					if spring.MatchTopics(consumB.Destination, prodB.Destination) {
						deps[producerApp] = true
						break // matched one out-binding from this producerApp, that's enough for a dependency
					}
				}
			}
		}

		var sortedDeps []string
		for dep := range deps {
			sortedDeps = append(sortedDeps, dep)
		}

		if len(sortedDeps) > 0 {
			graph[consumerApp] = sortedDeps
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(graph); err != nil {
		log.Fatalf("JSON encode: %v", err)
	}
}
