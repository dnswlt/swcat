package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/config"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/web"
	"gopkg.in/yaml.v3"
)

func formatFiles(files []string) error {
	for _, f := range files {
		log.Printf("Reading input file %s", f)
		es, err := store.ReadEntities(f)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", f, err)
		}
		if err := store.WriteEntities(f, es); err != nil {
			return err
		}
	}
	return nil
}

func lookupDotPath() string {
	path, err := exec.LookPath("dot")
	if err != nil && runtime.GOOS == "windows" {
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			// Try Graphviz default install path.
			candidate := filepath.Join(pf, "Graphviz", "bin", "dot.exe")
			if _, statErr := os.Stat(candidate); statErr == nil {
				path = candidate
				err = nil
			} else if matches, _ := filepath.Glob(filepath.Join(pf, "Graphviz*", "bin", "dot.exe")); len(matches) > 0 {
				// Installed in a specific version folder (e.g. Graphviz-12.0)
				path = matches[0]
				err = nil
			}
		}
	}

	if err != nil {
		log.Fatalf("dot was not found in your PATH. Please install Graphviz and add it to the PATH.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "-V")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run '%s -V': %v", path, err)
	}

	log.Printf("Found dot program at %s (%s)", path, strings.TrimSpace(string(output)))
	return path
}

func main() {

	serverAddrFlag := flag.String("addr", "localhost:8080", "Address to listen on")
	formatFlag := flag.Bool("format", false, "Format input files and exit.")
	baseDir := flag.String("base-dir", "", "Base directory for resource files. If empty, uses embedded resources.")
	configYaml := flag.String("config", "", "Path to the configuration YAML file")
	maxDepth := flag.Int("max-depth", 3, "Maximum recursion depth when scanning directories for .yml files")
	readOnly := flag.Bool("read-only", false, "Start server in read-only mode (no entity editing).")
	flag.Parse()

	// Check if dot (graphviz) is in the PATH, else try Windows default install path.
	dotPath := lookupDotPath()

	files, err := store.CollectYMLFiles(flag.Args(), *maxDepth)
	if err != nil {
		log.Fatalf("Failed to collect YAML files: %v", err)
	}

	if *formatFlag {
		err := formatFiles(files)
		if err != nil {
			log.Fatalf("Failed to format files: %v", err)
		}
		return
	}

	var bundle config.Bundle
	if *configYaml != "" {
		f, err := os.Open(*configYaml)
		if err != nil {
			log.Fatalf("Could not open configuration YAML: %v", err)
		}
		dec := yaml.NewDecoder(f)
		dec.KnownFields(true)
		if err := dec.Decode(&bundle); err != nil {
			log.Fatalf("Invalid configuration YAML: %v", err)
		}
	}

	repo, err := repo.LoadRepositoryFromPaths(bundle.Catalog, files)
	if err != nil {
		log.Fatalf("Failed to load repository: %v", err)
	}

	log.Printf("Read %d entities from %d files", repo.Size(), len(files))

	if *serverAddrFlag != "" {
		server, err := web.NewServer(
			web.ServerOptions{
				Addr:     *serverAddrFlag,
				BaseDir:  *baseDir,
				DotPath:  dotPath,
				ReadOnly: *readOnly,
				Config:   bundle,
			},
			repo,
		)
		if err != nil {
			log.Fatalf("Could not create server: %v", err)
		}
		log.Fatal(server.Serve()) // Never returns
	}

}
