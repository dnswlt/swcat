// bbclient is a small CLI for manually testing the Bitbucket Data Center client.
//
// Usage:
//
//	bbclient -url https://bb.example.com -project PROJ -repo myrepo commits
//	bbclient -url https://bb.example.com -project PROJ -repo myrepo cat path/to/file.go
//	bbclient -url https://bb.example.com -project PROJ -repo myrepo cat path/to/file.go -at main
//
// Credentials can be supplied via -user / -pass flags or the BBCLIENT_USER /
// BBCLIENT_PASS environment variables.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dnswlt/swcat/internal/plugins/bitbucket"
)

func main() {
	var (
		baseURL    string
		projectKey string
		repoSlug   string
		username   string
		password   string
	)

	flag.StringVar(&baseURL, "url", "", "Bitbucket base URL (e.g. https://bitbucket.example.com)")
	flag.StringVar(&projectKey, "project", "", "Bitbucket project key")
	flag.StringVar(&repoSlug, "repo", "", "Repository slug")
	flag.StringVar(&username, "user", os.Getenv("BBCLIENT_USER"), "Username (or set BBCLIENT_USER)")
	flag.StringVar(&password, "pass", os.Getenv("BBCLIENT_PASS"), "Password or token (or set BBCLIENT_PASS)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: bbclient -url URL -project KEY -repo SLUG <command> [args]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  commits              Print recent commits\n")
		fmt.Fprintf(os.Stderr, "  cat <file> [-at REV] Print file contents at optional revision\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if baseURL == "" || projectKey == "" || repoSlug == "" {
		fmt.Fprintln(os.Stderr, "Error: -url, -project, and -repo are required")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: a command is required (commits or cat)")
		flag.Usage()
		os.Exit(1)
	}

	client := bitbucket.NewClient(baseURL, username, password)
	ctx := context.Background()

	switch args[0] {
	case "commits":
		runCommits(ctx, client, projectKey, repoSlug, args[1:])
	case "cat":
		runCat(ctx, client, projectKey, repoSlug, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}

func runCommits(ctx context.Context, client *bitbucket.Client, projectKey, repoSlug string, args []string) {
	fs := flag.NewFlagSet("commits", flag.ExitOnError)
	until := fs.String("until", "", "Show commits reachable from this ref or commit ID")
	since := fs.String("since", "", "Show commits after this ref or commit ID (exclusive)")
	limit := fs.Int("limit", 10, "Maximum number of commits to show")
	fs.Parse(args)

	commits, err := client.GetCommits(ctx, projectKey, repoSlug, bitbucket.GetCommitsOptions{
		Until: *until,
		Since: *since,
		Limit: *limit,
	})
	if err != nil {
		log.Fatalf("GetCommits: %v", err)
	}

	if len(commits) == 0 {
		fmt.Println("No commits found.")
		return
	}
	for _, c := range commits {
		fmt.Printf("%s  %s <%s>  %s\n\t%s\n",
			c.DisplayID,
			c.Author.Name,
			c.Author.EmailAddress,
			c.AuthorTime().Format("2006-01-02 15:04:05"),
			c.Message,
		)
	}
}

func runCat(ctx context.Context, client *bitbucket.Client, projectKey, repoSlug string, args []string) {
	fs := flag.NewFlagSet("cat", flag.ExitOnError)
	at := fs.String("at", "", "Revision (branch, tag, or commit ID); defaults to default branch")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Error: cat requires a file path argument")
		os.Exit(1)
	}
	filePath := fs.Arg(0)

	data, err := client.GetFileContents(ctx, projectKey, repoSlug, filePath, *at)
	if err != nil {
		log.Fatalf("GetFileContents: %v", err)
	}

	os.Stdout.Write(data)
}
