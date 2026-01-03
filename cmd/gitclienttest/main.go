package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dnswlt/swcat/internal/gitclient"
)

func main() {
	var (
		url      string
		username string
		password string
		ref      string
	)

	flag.StringVar(&url, "url", "", "Repository URL to list")
	flag.StringVar(&username, "user", "", "Username for authentication")
	flag.StringVar(&password, "pass", "", "Password or Token for authentication")
	flag.StringVar(&ref, "ref", "main", "Reference (branch or tag) to list files from")
	flag.Parse()

	if url == "" {
		fmt.Println("Error: -url is required")
		flag.Usage()
		os.Exit(1)
	}

	var auth *gitclient.Auth
	if username != "" || password != "" {
		auth = &gitclient.Auth{
			Username: username,
			Password: password,
		}
	}

	loader, err := gitclient.NewCatalogLoader(url, auth)
	if err != nil {
		log.Fatalf("Failed to create loader for %q: %v", url, err)
	}

	// List branches and tags
	refs, err := loader.ListReferences()
	if err != nil {
		log.Fatalf("Failed to list references: %v", err)
	}
	if len(refs) == 0 {
		log.Fatalf("No branches or tags found in %q", url)
	}

	fmt.Printf("Branches and tags in %s:\n", url)
	for _, v := range refs {
		fmt.Printf("  %s\n", v)
	}

	// List files for the specified revision
	files, err := loader.ListFilesRecursive(ref, "")
	if err != nil {
		log.Fatalf("Failed to list files for revision %q: %v", ref, err)
	}

	fmt.Printf("\nFiles at revision %q:\n", ref)
	for _, f := range files {
		fmt.Printf("  %s\n", f)
	}
}
