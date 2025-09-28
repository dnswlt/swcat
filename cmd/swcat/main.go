package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dnswlt/swcat/internal/backstage"
	"github.com/dnswlt/swcat/internal/web"
)

// collectYMLFilesInDir walks root recursively up to maxDepth levels below root
// (root itself is depth 0) and returns all *.yml files it finds.
// It does NOT follow symlinks. It skips directories deeper than maxDepth.
func collectYMLFilesInDir(root string, maxDepth int) ([]string, error) {
	root = filepath.Clean(root)
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // propagate filesystem error
		}

		if d.IsDir() {
			// Compute depth relative to root (root=0, its children=1, etc.)
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			depth := strings.Count(rel, string(os.PathSeparator)) + 1
			if depth > maxDepth {
				return fs.SkipDir
			}
			return nil
		}

		// Match *.yml (case-insensitive)
		if strings.HasSuffix(strings.ToLower(d.Name()), ".yml") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(out) // deterministic order
	return out, nil
}

func collectYMLFiles(args []string) ([]string, error) {
	var allFiles []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %v", arg, err)
		}

		if info.IsDir() {
			// Collect files recursively, up to 3 levels deep
			files, err := collectYMLFilesInDir(arg, 3)
			if err != nil {
				return nil, fmt.Errorf("failed to walk dir %s: %v", arg, err)
			}
			allFiles = append(allFiles, files...)
		} else {
			allFiles = append(allFiles, arg)
		}
	}
	return allFiles, nil

}

func main() {

	serverAddrFlag := flag.String("addr", "localhost:8080", "Address to listen on")
	baseDir := flag.String("base-dir", ".", "Base directory")
	flag.Parse()

	// Check if dot (graphviz) is in the PATH, else abort.
	// We need dot to render SVG graphs.
	path, err := exec.LookPath("dot")
	if err != nil {
		log.Fatalf("dot was not found in your PATH. Please install Graphviz and add it to the PATH.")
	}
	log.Printf("Found dot program at %s", path)

	repo := backstage.NewRepository()

	files, err := collectYMLFiles(flag.Args())
	if err != nil {
		log.Fatalf("Failed to collect YAML files: %v", err)
	}

	for _, arg := range files {
		log.Printf("Reading input file %s", arg)
		es, err := backstage.ReadEntities(arg)
		if err != nil {
			log.Fatalf("Failed to read %s: %v", arg, err)
		}
		for _, e := range es {
			err = repo.AddEntity(e)
			if err != nil {
				log.Fatalf("Failed to add entity %s to repository: %v", e.GetQName(), err)
			}
		}
	}

	log.Printf("Read %d entities from %d files", repo.Size(), len(files))

	if err := repo.Validate(); err != nil {
		log.Fatalf("Repository validation failed: %v", err)
	}
	log.Println("Entity validation successful")

	if *serverAddrFlag != "" {
		server, err := web.NewServer(
			web.ServerOptions{
				Addr:    *serverAddrFlag,
				BaseDir: *baseDir,
			},
			repo,
		)
		if err != nil {
			log.Fatalf("Could not create server: %v", err)
		}
		log.Fatal(server.Serve()) // Never returns
	}

}
