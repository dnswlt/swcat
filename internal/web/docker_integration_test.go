//go:build docker

package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/net/html"
)

// extractLinks parses the HTML and collects URLs from various HTML and HTMX attributes.
func extractLinks(body io.Reader) ([]string, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return nil, err
	}
	var links []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				key := strings.ToLower(a.Key)
				val := strings.TrimSpace(a.Val)
				if val == "" {
					continue
				}

				switch key {
				case "href", "src", "action", "data", "poster":
					links = append(links, val)
				case "hx-get":
					links = append(links, val)
				case "hx-push-url":
					if val != "true" && val != "false" {
						links = append(links, val)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return links, nil
}

func TestDockerIntegration_AssetsSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	// Locate project root relative to this test file (internal/web/ -> root)
	projectRoot, err := filepath.Abs("../../")
	if err != nil {
		t.Fatalf("Failed to resolve project root: %v", err)
	}

	flightsPath := filepath.Join(projectRoot, "examples", "flights")

	imageName := os.Getenv("SWCAT_TEST_IMAGE")
	if imageName == "" {
		imageName = "swcat:latest"
	}

	// Start container directly with testcontainers-go using a pre-built image.
	req := testcontainers.ContainerRequest{
		Image:        imageName,
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"SWCAT_ADDR":     "0.0.0.0:8080",
			"SWCAT_ROOT_DIR": "/data",
		},
		Binds: []string{
			fmt.Sprintf("%s:/data:ro", flightsPath),
		},
		WaitingFor: wait.ForHTTP("/status").WithPort("8080/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}
	defer func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		if err := container.Terminate(termCtx); err != nil {
			t.Logf("Warning: failed to terminate container: %v", err)
		}
	}()

	// Get mapped port
	ip, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get host: %v", err)
	}
	port, err := container.MappedPort(ctx, "8080")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", ip, port.Port())
	t.Logf("Container is running at: %s", baseURL)

	// List of pages to inspect and extract resources from
	pagesToCheck := []string{
		"/ui/components",
		"/ui/systems/flights-search",
	}

	checkedResources := make(map[string]bool)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for _, pagePath := range pagesToCheck {
		pageURL := baseURL + pagePath
		t.Run(fmt.Sprintf("Page_%s", strings.ReplaceAll(pagePath, "/", "_")), func(t *testing.T) {
			resp, err := client.Get(pageURL)
			if err != nil {
				t.Fatalf("Failed to GET page %s: %v", pageURL, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Page %s returned status %d, expected 200 OK", pageURL, resp.StatusCode)
			}

			links, err := extractLinks(resp.Body)
			if err != nil {
				t.Fatalf("Failed to parse HTML from page %s: %v", pageURL, err)
			}

			baseParsed, _ := url.Parse(pageURL)

			for _, link := range links {
				// Skip empty references, anchors, and data URIs
				trimmedLink := strings.TrimSpace(link)
				if trimmedLink == "" || strings.HasPrefix(trimmedLink, "#") || strings.HasPrefix(trimmedLink, "data:") {
					continue
				}

				refParsed, err := url.Parse(trimmedLink)
				if err != nil {
					t.Errorf("Failed to parse resource link %q: %v", trimmedLink, err)
					continue
				}

				resolved := baseParsed.ResolveReference(refParsed)

				// Ensure we only verify localhost/internal resources served by the container
				if resolved.Host != baseParsed.Host {
					continue
				}
				if resolved.Scheme != "http" && resolved.Scheme != "https" {
					continue
				}

				resolvedStr := resolved.String()
				if checkedResources[resolvedStr] {
					continue
				}
				checkedResources[resolvedStr] = true

				t.Run(fmt.Sprintf("Resource_%s", resolved.Path), func(t *testing.T) {
					resResp, err := client.Get(resolvedStr)
					if err != nil {
						t.Errorf("Failed to fetch resource %s: %v", resolvedStr, err)
						return
					}
					defer resResp.Body.Close()

					if resResp.StatusCode == http.StatusNotFound || resResp.StatusCode >= http.StatusInternalServerError {
						t.Errorf("Resource %s returned status %d, expected it to be available (not 404 or 5xx)", resolvedStr, resResp.StatusCode)
					} else {
						t.Logf("Successfully verified resource: %s (status %d)", resolved.Path, resResp.StatusCode)
					}
				})
			}
		})
	}
}
