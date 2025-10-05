package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/backstage"
	"github.com/dnswlt/swcat/internal/web"
)

func formatFiles(files []string) error {
	for _, f := range files {
		log.Printf("Reading input file %s", f)
		es, err := backstage.ReadEntities(f)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", f, err)
		}
		if err := backstage.WriteEntities(f, es); err != nil {
			return err
		}
	}
	return nil
}

func main() {

	serverAddrFlag := flag.String("addr", "localhost:8080", "Address to listen on")
	formatFlag := flag.Bool("format", false, "Format input files and exit. This currently remvoes all comments!")
	baseDir := flag.String("base-dir", ".", "Base directory")
	flag.Parse()

	// Check if dot (graphviz) is in the PATH, else abort.
	// We need dot to render SVG graphs.
	func() {
		path, err := exec.LookPath("dot")
		if err != nil {
			log.Fatalf("dot was not found in your PATH. Please install Graphviz and add it to the PATH.")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dot", "-V")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// This error is returned if the command cannot be found or exits with a non-zero status.
			log.Fatalf("Failed to run 'dot -V': %v", err)
		}
		log.Printf("Found dot program at %s (%s)", path, strings.TrimSpace(string(output)))
	}()

	files, err := backstage.CollectYMLFiles(flag.Args(), 3) // max 3 directory levels deep.
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

	repo, err := backstage.LoadRepositoryFromPaths(files)
	if err != nil {
		log.Fatalf("Failed to load repository: %v", err)
	}

	log.Printf("Read %d entities from %d files", repo.Size(), len(files))

	if *serverAddrFlag != "" {
		server, err := web.NewServer(
			web.ServerOptions{
				Addr:    *serverAddrFlag,
				BaseDir: *baseDir,
				DotPath: "dot",
			},
			repo,
		)
		if err != nil {
			log.Fatalf("Could not create server: %v", err)
		}
		log.Fatal(server.Serve()) // Never returns
	}

}
