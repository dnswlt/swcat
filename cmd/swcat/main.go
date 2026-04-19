package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	iofs "io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/comments"
	"github.com/dnswlt/swcat/internal/database"
	"github.com/dnswlt/swcat/internal/gitclient"
	"github.com/dnswlt/swcat/internal/jfrog"
	"github.com/dnswlt/swcat/internal/kube"
	"github.com/dnswlt/swcat/internal/lint"
	"github.com/dnswlt/swcat/internal/maven"
	"github.com/dnswlt/swcat/internal/plugins"
	"github.com/dnswlt/swcat/internal/prometheus"
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
		return "", fmt.Errorf("dot was not found in your PATH. Please install Graphviz (https://graphviz.org/) and add it to the PATH: %v", err)
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

func promClientAuthFromEnv() (opts prometheus.ClientOptions) {
	opts.Username = os.Getenv("SWCAT_PROMETHEUS_USER")
	opts.Password = os.Getenv("SWCAT_PROMETHEUS_PASSWORD")
	opts.BearerToken = os.Getenv("SWCAT_PROMETHEUS_TOKEN")
	return opts
}

func jfrogClientAuthFromEnv() (jfrog.Auth, error) {
	mvnSettingsPath := os.Getenv("SWCAT_JFROG_MAVEN_SETTINGS_PATH")
	if mvnSettingsPath == "" {
		return jfrog.Auth{
			Username: os.Getenv("SWCAT_JFROG_USER"),
			Password: os.Getenv("SWCAT_JFROG_PASSWORD"),
		}, nil
	}

	settings, err := maven.ReadSettings(mvnSettingsPath)
	if err != nil {
		return jfrog.Auth{}, err
	}
	serverID := os.Getenv("SWCAT_JFROG_MAVEN_SERVER_ID")
	server, err := settings.ServerByID(serverID)
	if err != nil {
		return jfrog.Auth{}, err
	}
	return jfrog.Auth{
		Username: server.Username,
		Password: server.Password,
	}, nil
}

func createJFrogClient(opts *Options) (jfrog.Client, error) {
	if opts.JFrogURL == "" {
		return nil, nil
	}
	auth, err := jfrogClientAuthFromEnv()
	if err != nil {
		return nil, err
	}
	cfg := jfrog.Config{
		JFrogURL: opts.JFrogURL,
		Auth:     auth,
		Timeout:  5 * time.Second,
	}
	return jfrog.NewClient(cfg), nil
}

func createPluginRegistry(source store.Source, services plugins.Services) (*plugins.Registry, error) {
	st, err := source.Store("")
	if err != nil {
		return nil, fmt.Errorf("could not get default store: %w", err)
	}
	data, err := st.ReadFile(store.PluginsFile)
	if errors.Is(err, iofs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to load plugins config: %w", err)
	}
	config, err := plugins.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	return plugins.NewRegistry(config, services)
}

// Options contains program options that can be set via command-line flags or environment variables.
type Options struct {
	Addr              string
	RootDir           string
	GitURL            string
	GitRef            string
	GitRootDir        string
	GitUserName       string
	GitUserEmail      string
	BaseDir           string
	ReadOnly          bool
	DisablePlugins    bool
	DotTimeout        time.Duration
	UseDotStreaming   bool
	SVGCacheSize      int
	CommentsDir       string
	KubeKubeconfig    string
	KubeContext       string
	KubeInCluster     bool
	PrometheusURL     string
	PrometheusTimeout time.Duration
	BitbucketURL      string
	JFrogURL          string
	DocumentsDir      string
	DatabaseDSN       string
}

func createKubeClient(source store.Source, opts *Options) (kube.Client, error) {
	cc := kube.ConnectConfig{
		Kubeconfig: opts.KubeKubeconfig,
		Context:    opts.KubeContext,
		InCluster:  opts.KubeInCluster,
	}
	if cc.Kubeconfig == "" && !cc.InCluster {
		return nil, nil // Kube not configured, not an error.
	}
	defaultStore, err := source.Store("")
	if err != nil {
		return nil, fmt.Errorf("could not get default store: %w", err)
	}
	kubeData, err := defaultStore.ReadFile(store.KubeFile)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil, fmt.Errorf("kube connection configured but no %s config file found", store.KubeFile)
		}
		return nil, fmt.Errorf("could not load kube config: %w", err)
	}
	cfg, err := kube.ParseConfig(kubeData)
	if err != nil {
		return nil, fmt.Errorf("could not parse kube config: %w", err)
	}
	client, err := kube.NewClientFromConfig(cc, *cfg)
	if err != nil {
		return nil, fmt.Errorf("could not create Kubernetes client: %w", err)
	}
	return client, nil
}

func createLinter(source store.Source) (*lint.Linter, error) {
	st, err := source.Store("")
	if err != nil {
		return nil, fmt.Errorf("could not get default store: %w", err)
	}
	lintYaml, err := st.ReadFile(store.LintFile)
	if errors.Is(err, iofs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to load lint config: %w", err)
	}
	lintCfg, err := lint.ParseConfig(lintYaml)
	if err != nil {
		return nil, err
	}
	return lint.NewLinter(lintCfg)
}

func createPrometheusClient(opts *Options) *prometheus.Client {
	if opts.PrometheusURL == "" {
		return nil
	}
	clientOpts := promClientAuthFromEnv()
	clientOpts.Timeout = opts.PrometheusTimeout
	return prometheus.NewClient(opts.PrometheusURL, clientOpts)
}

func createBitbucketClient(opts *Options) *bitbucket.Client {
	if opts.BitbucketURL == "" {
		return nil
	}
	username := os.Getenv("SWCAT_BITBUCKET_USER")
	password := os.Getenv("SWCAT_BITBUCKET_PASSWORD")
	tokens := make(map[string]string)
	const prefix = "SWCAT_BITBUCKET_TOKEN_"
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, prefix) {
			key, value, _ := strings.Cut(v, "=")
			proj := strings.TrimPrefix(key, prefix)
			tokens[strings.ToLower(proj)] = value
		}
	}
	client := bitbucket.NewClient(opts.BitbucketURL, bitbucket.ClientOptions{
		Username:         username,
		Password:         password,
		PerProjectTokens: tokens,
	})
	return client
}

func ensureSqliteDir(dsn string) error {
	// Strip query parameters and remove the standard SQLite URI scheme.
	p := dsn
	p, _, _ = strings.Cut(p, "?")
	p = strings.TrimPrefix(p, "file:")

	// Create directory if necessary.
	if p == ":memory:" || p == "" {
		return nil
	}
	dir := filepath.Dir(p)
	if dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func main() {

	var opts Options
	fs := flag.NewFlagSet("swcat", flag.ContinueOnError)
	fs.StringVar(&opts.Addr, "addr", "localhost:8080", "Address to listen on")
	fs.StringVar(&opts.RootDir, "root-dir", ".", "Root directory of the local data store")
	fs.StringVar(&opts.GitURL, "git-url", "", "URL of the git repository to use as the data store")
	fs.StringVar(&opts.GitRef, "git-ref", "", "Git ref (branch or tag) to use initially")
	fs.StringVar(&opts.GitRootDir, "git-root-dir", ".", "Path to the directory within the git repository that contains the catalog structure")
	fs.StringVar(&opts.GitUserName, "git-user-name", "", "Name used for git commits in edit sessions")
	fs.StringVar(&opts.GitUserEmail, "git-user-email", "", "Email used for git commits in edit sessions")
	fs.StringVar(&opts.BaseDir, "base-dir", "", "Base directory for resource files. If empty, uses embedded resources (recommended for production).")
	fs.BoolVar(&opts.ReadOnly, "read-only", false, "Start server in read-only mode (no entity editing).")
	fs.BoolVar(&opts.DisablePlugins, "disable-plugins", false, "Disable all plugins (even if a plugin config is found)")
	fs.DurationVar(&opts.DotTimeout, "dot-timeout", 10*time.Second, "Maximum time to wait before cancelling dot executions")
	fs.BoolVar(&opts.UseDotStreaming, "dot-streaming", runtime.GOOS == "windows", "Use long-running dot process to render SVG graphs (use only if dot process startup is slow, e.g. on Windows)")
	fs.IntVar(&opts.SVGCacheSize, "svg-cache-size", 1024, "Max. number of SVG graphs to hold in the in-memory LRU cache")
	fs.StringVar(&opts.CommentsDir, "comments-dir", "", "Directory where entity comments are stored (relative to root-dir if not absolute). If empty, comments are disabled.")
	fs.StringVar(&opts.KubeKubeconfig, "kube-kubeconfig", "", "Path to the kubeconfig file for Kubernetes workload scanning")
	fs.StringVar(&opts.KubeContext, "kube-context", "", "Kubernetes context to use (only with -kube-kubeconfig)")
	fs.BoolVar(&opts.KubeInCluster, "kube-in-cluster", false, "Use in-cluster Kubernetes config (for running inside a pod)")
	fs.StringVar(&opts.PrometheusURL, "prometheus-url", "", "Base URL of a Prometheus or Thanos REST endpoint (for linting)")
	fs.DurationVar(&opts.PrometheusTimeout, "prometheus-timeout", 30*time.Second, "Maximum time to wait for Prometheus queries")
	fs.StringVar(&opts.BitbucketURL, "bitbucket-url", "", "Base URL of the Bitbucket Data Center instance (e.g. https://bitbucket.example.com)")
	fs.StringVar(&opts.JFrogURL, "jfrog-url", "", "Base URL of the JFrog Artifactory/Docker repository")
	fs.StringVar(&opts.DocumentsDir, "documents-dir", "", "Local path to serve HTML documents from, bypassing the catalog store (e.g. for sidecar sync)")
	fs.StringVar(&opts.DatabaseDSN, "database-dsn", "", "SQLite DSN for entity status observations (e.g. data.db)")

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

	var source store.Source
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
		gitSource := store.NewGitSource(loader, ref, opts.GitRootDir, gitclient.Author{
			Name:  opts.GitUserName,
			Email: opts.GitUserEmail,
		})
		if restored, err := gitSource.RestoreSessions(); err != nil {
			log.Printf("Warning: failed to restore edit sessions: %v", err)
		} else if l := len(restored); l > 0 {
			log.Printf("Restored %d edit/ sessions from remote branches", l)
		}
		source = gitSource
	} else if opts.RootDir != "" {
		log.Printf("Using local store at %s", opts.RootDir)
		source = store.NewDiskStore(opts.RootDir)
	} else {
		log.Fatalf("Neither -root-dir nor -git-url specified")
	}

	// Load optional linter
	linter, err := createLinter(source)
	if err != nil {
		log.Fatalf("Failed to create linter: %v", err)
	} else if linter != nil {
		log.Printf("Linter initialized from %s with %d rules", store.LintFile, linter.NumRules())
	}

	var commentsStore comments.Store
	if opts.CommentsDir != "" {
		commentsDir := opts.CommentsDir
		if !filepath.IsAbs(commentsDir) {
			commentsDir = filepath.Join(opts.RootDir, commentsDir)
		}
		fileCommentsStore, err := comments.NewFileStore(commentsDir)
		if err != nil {
			log.Fatalf("Could not create comments store: %v", err)
		}
		commentsStore = comments.NewCachingStore(fileCommentsStore)
	}

	// Optionally create a Kubernetes client.
	kubeClient, err := createKubeClient(source, &opts)
	if err != nil {
		// Do not fail here, k8s support is truly optional.
		log.Printf("Could not create kube client: %v", err)
	} else if kubeClient != nil {
		log.Printf("Kubernetes client initialized (kubeconfig=%s, in-cluster=%v)", opts.KubeKubeconfig, opts.KubeInCluster)
	}

	// Optionally create a Prometheus scanner.
	promClient := createPrometheusClient(&opts)
	if promClient != nil {
		log.Printf("Prometheus client initialized")
	}

	// Optionally create a Bitbucket client.
	bbClient := createBitbucketClient(&opts)
	if bbClient != nil {
		log.Printf("Bitbucket client initialized (url=%s)", opts.BitbucketURL)
	}

	// Optionally initialize SQLite database
	var db *sql.DB
	if opts.DatabaseDSN != "" {
		if err := ensureSqliteDir(opts.DatabaseDSN); err != nil {
			log.Fatalf("Failed to create directory for database: %v", err)
		}

		db = database.NewSqlite(opts.DatabaseDSN)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := database.RecreateTables(ctx, db, false)
		cancel()
		if err != nil {
			log.Fatalf("Failed to initialize database tables in %s: %v", opts.DatabaseDSN, err)
		}
		log.Printf("Database initialized from %s", opts.DatabaseDSN)
	}

	jfrogClient, err := createJFrogClient(&opts)
	if err != nil {
		log.Fatalf("Failed to create JFrog client: %v", err)
	} else if jfrogClient != nil {
		log.Printf("Created JFrog client for %s", opts.JFrogURL)
	}

	var pluginRegistry *plugins.Registry
	if !opts.ReadOnly && !opts.DisablePlugins {
		r, err := createPluginRegistry(source, plugins.Services{
			JFrogClient: jfrogClient,
		})
		if err != nil {
			log.Fatalf("Could not create plugin registry: %v", err)
		} else if r != nil {
			log.Printf("%d plugins initialized from %s", len(r.Plugins()), store.PluginsFile)
		}
		pluginRegistry = r
	}

	server, err := web.NewServer(
		web.ServerOptions{
			Addr:            opts.Addr,
			BaseDir:         opts.BaseDir,
			DotPath:         dotPath,
			DotTimeout:      opts.DotTimeout,
			UseDotStreaming: opts.UseDotStreaming,
			ReadOnly:        opts.ReadOnly,
			Version:         Version,
			SVGCacheSize:    opts.SVGCacheSize,
			DocumentsDir:    opts.DocumentsDir,
		},
		source,
		web.WithLinter(linter),
		web.WithPluginRegistry(pluginRegistry),
		web.WithCommentsStore(commentsStore),
		web.WithKubeClient(kubeClient),
		web.WithPrometheusClient(promClient),
		web.WithBitbucketClient(bbClient),
		web.WithDatabase(db),
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
