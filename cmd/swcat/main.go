package main

import (
	"flag"
	"log"
	"os/exec"

	"github.com/dnswlt/swcat/internal/backstage"
	"github.com/dnswlt/swcat/internal/web"
)

func main() {

	serverAddrFlag := flag.String("addr", "localhost:8080", "Address to listen on")
	baseDir := flag.String("base-dir", ".", "Base directory")
	flag.Parse()

	// Check if dot (graphviz) is in the PATH, else abort.
	// We need dot to render SVG graphs.
	path, err := exec.LookPath("dot")
	if err != nil {
		log.Fatalf("dot was not found in your PATH")
	}
	log.Printf("Found dot program at %s", path)

	repo := backstage.NewRepository()

	for _, arg := range flag.Args() {
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

	log.Printf("Read %d entities from %d files", repo.Size(), len(flag.Args()))

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
