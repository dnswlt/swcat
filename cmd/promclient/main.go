// promclient is a small CLI for manually testing the Prometheus/Thanos client.
//
// Usage:
//
//	promclient -url https://prometheus.example.com 'up{job="kube-state-metrics"}'
//	promclient -url https://thanos.example.com -token $TOKEN 'kube_pod_container_status_ready{namespace="prod"}'
//
// Credentials can be supplied via flags or environment variables:
//
//	PROMCLIENT_URL, PROMCLIENT_TOKEN, PROMCLIENT_USER, PROMCLIENT_PASS
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/dnswlt/swcat/internal/prometheus"
)

func main() {
	var (
		baseURL  string
		token    string
		username string
		password string
	)

	flag.StringVar(&baseURL, "url", os.Getenv("PROMCLIENT_URL"), "Prometheus/Thanos base URL (or set PROMCLIENT_URL)")
	flag.StringVar(&token, "token", os.Getenv("PROMCLIENT_TOKEN"), "Bearer token (or set PROMCLIENT_TOKEN)")
	flag.StringVar(&username, "user", os.Getenv("PROMCLIENT_USER"), "Username for basic auth (or set PROMCLIENT_USER)")
	flag.StringVar(&password, "pass", os.Getenv("PROMCLIENT_PASS"), "Password for basic auth (or set PROMCLIENT_PASS)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: promclient [flags] <PromQL query>\n\n")
		fmt.Fprintf(os.Stderr, "Executes an instant PromQL query and prints each resulting sample.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "Error: -url is required")
		flag.Usage()
		os.Exit(1)
	}
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Error: a PromQL query argument is required")
		flag.Usage()
		os.Exit(1)
	}
	query := strings.Join(flag.Args(), " ")

	client := prometheus.NewClient(baseURL, prometheus.ClientOptions{
		BearerToken: token,
		Username:    username,
		Password:    password,
	})

	samples, err := client.Query(context.Background(), query)
	if err != nil {
		log.Fatalf("Query: %v", err)
	}

	if len(samples) == 0 {
		fmt.Println("No results.")
		return
	}

	for _, s := range samples {
		fmt.Printf("%s  %s\n", formatLabels(s.Labels), formatValue(s.Value))
	}
}

// formatLabels prints metric labels in the standard Prometheus notation,
// with __name__ promoted to the front and the rest sorted alphabetically.
func formatLabels(labels map[string]string) string {
	name := labels["__name__"]
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k != "__name__" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%q", k, labels[k]))
	}

	if name != "" {
		return fmt.Sprintf("%s{%s}", name, strings.Join(pairs, ", "))
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

// formatValue formats a float sample value, using integer notation when the
// value is a whole number and the special names NaN/+Inf/-Inf otherwise.
func formatValue(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case v == math.Trunc(v):
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%g", v)
	}
}
