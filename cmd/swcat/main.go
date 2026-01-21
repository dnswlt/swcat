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

	"github.com/dnswlt/swcat/internal/gitclient"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/web"
	"github.com/peterbourgon/ff/v3"
)

func lookupDotPath() (string, error) {
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
		return "", fmt.Errorf("dot was not found in your PATH. Please install Graphviz and add it to the PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "-V")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run '%s -V': %v", path, err)
	}

	log.Printf("Found dot program at %s (%s)", path, strings.TrimSpace(string(output)))
	return path, nil
}

var (
	// Version is the application version.
	// It is set at build time via -ldflags "-X main.Version=...".
	Version = "dev"
)

func gitClientAuthFromEnv() *gitclient.Auth {
	user := os.Getenv("SWCAT_GIT_USER")
	if user == "" {
		return nil
	}
	pass := os.Getenv("SWCAT_GIT_PASSWORD")
	return &gitclient.Auth{
		Username: user,
		Password: pass,
	}
}

// Options contains program options that can be set via command-line flags or environment variables.
type Options struct {
	Addr            string
	CatalogDir      string
	RootDir         string
	GitURL          string
	GitRef          string
	ConfigFile      string
	BaseDir         string
	ReadOnly        bool
	DotTimeout      time.Duration
	UseDotStreaming bool
}

func main() {

	var opts Options
	fs := flag.NewFlagSet("swcat", flag.ContinueOnError)
	fs.StringVar(&opts.Addr, "addr", "localhost:8080", "Address to listen on")
	fs.StringVar(&opts.RootDir, "root-dir", ".", "Root directory of the local data store")
	fs.StringVar(&opts.CatalogDir, "catalog-dir", "catalog", "Path to the catalog directory containing YAML files (relative to git root or local -root-dir)")
	fs.StringVar(&opts.ConfigFile, "config", "swcat.yml", "Path to the configuration YAML file (relative to git root or local -root-dir)")
	fs.StringVar(&opts.GitURL, "git-url", "", "URL of the git repository to use as the data store")
	fs.StringVar(&opts.GitRef, "git-ref", "", "Git ref (branch or tag) to use initially")
	fs.StringVar(&opts.BaseDir, "base-dir", "", "Base directory for resource files. If empty, uses embedded resources (recommended for production).")
	fs.BoolVar(&opts.ReadOnly, "read-only", false, "Start server in read-only mode (no entity editing).")
	fs.DurationVar(&opts.DotTimeout, "dot-timeout", 10*time.Second, "Maximum time to wait before cancelling dot executions")
	fs.BoolVar(&opts.UseDotStreaming, "dot-streaming", runtime.GOOS == "windows", "Use long-running dot process to render SVG graphs (use only if dot process startup is slow, e.g. on Windows)")

	err := ff.Parse(fs, os.Args[1:], ff.WithEnvVarPrefix("SWCAT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		os.Exit(1)
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "Unexpected positional arguments: %v\n", fs.Args())
		os.Exit(1)
	}
	log.Printf("Using config from flags/env vars: %+v", opts)

	// Check if dot (graphviz) is in the PATH, else try Windows default install path.
	dotPath, err := lookupDotPath()
	if err != nil {
		log.Fatalf("dot was not found in your PATH. Please install Graphviz and add it to the PATH.")
	}

	var st store.Source
	if opts.GitURL != "" {
		auth := gitClientAuthFromEnv()
		log.Printf("Retrieving catalog from git URL %s", opts.GitURL)
		loader, err := gitclient.New(opts.GitURL, auth)
		if err != nil {
			log.Fatalf("Failed to retrieve git repo: %v", err)
		}
		ref := opts.GitRef
		if ref == "" {
			ref, err = loader.DefaultBranch()
			if err != nil {
				log.Fatalf("No git-ref specified and no default branch found: %v", err)
			}
		}
		log.Printf("Using default git branch %q", ref)
		st = store.NewGitSource(loader, ref)
		if !opts.ReadOnly {
			opts.ReadOnly = true // Enforce read-only mode when using a remote git repo as the store.
			log.Printf("Activated read-only mode for git-based storage")
		}
	} else if opts.RootDir != "" {
		log.Printf("Using local store at %s", opts.RootDir)
		st = store.NewDiskStore(opts.RootDir)
	} else {
		log.Fatalf("Neither -root-dir nor -git-url specified")
	}

	server, err := web.NewServer(
		web.ServerOptions{
			Addr:            opts.Addr,
			BaseDir:         opts.BaseDir,
			CatalogDir:      opts.CatalogDir,
			DotPath:         dotPath,
			DotTimeout:      opts.DotTimeout,
			UseDotStreaming: opts.UseDotStreaming,
			ReadOnly:        opts.ReadOnly,
			ConfigFile:      opts.ConfigFile,
			Version:         Version,
		},
		st,
	)
	if err != nil {
		log.Fatalf("Could not create server: %v", err)
	}

	// Ensure the repo in the default ref can be read.
	// Otherwise it's pointless to even start the server.
	size, err := server.ValidateCatalog("")
	if err != nil {
		log.Fatalf("Could not load default catalog: %v", err)
	}
	log.Printf("Read %d entities from catalog", size)

	log.Fatal(server.Serve()) // Never returns

}
