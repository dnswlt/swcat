// Package main implements a simple launcher and auto-updater for swcat.
//
// The launcher follows these conventions:
//  1. It scans the current directory for subfolders matching "swcat-v*" to find the latest
//     locally installed version.
//  2. It checks the GitHub "latest" release for a newer version.
//  3. It identifies the correct download asset by looking for a .zip file containing
//     the current OS (GOOS) and architecture (GOARCH) in its filename.
//  4. It expects the downloaded .zip to contain a single top-level folder named "swcat-<version>/",
//     which is extracted into the launcher's directory.
//  5. It then identifies the newest local version again and launches the "swcat" (or "swcat.exe")
//     binary found within that version's folder, passing through all command-line arguments.
//
// This approach allows for simple "drop-in" updates without requiring complex installation
// procedures or elevated privileges.
package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// GithubRelease represents the minimal JSON structure we need
type GithubRelease struct {
	TagName string `json:"tag_name"` // e.g., "v1.2.3"
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

const (
	releasesLatestURL = "https://api.github.com/repos/dnswlt/swcat/releases/latest"
)

// getLatestRelease queries GitHub and returns the latest tag name and the download URL for the current platform.
func getLatestRelease() (string, string, error) {
	// Create Request with timeout (Critical for a launcher!)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", releasesLatestURL, nil)
	if err != nil {
		return "", "", err
	}

	// 3. Execute
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	// 4. Handle Rate Limits & Errors gracefully
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github api returned status: %d", resp.StatusCode)
	}

	// 5. Parse
	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", fmt.Errorf("json decode error: %w", err)
	}

	// 6. Find the correct asset for the OS/Arch
	targetArch := runtime.GOARCH // e.g., "amd64"
	targetOS := runtime.GOOS     // e.g., "windows"

	var names []string
	for _, asset := range release.Assets {
		names = append(names, asset.Name)
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, targetOS) && strings.Contains(name, targetArch) {
			return release.TagName, asset.BrowserDownloadURL, nil
		}
	}

	return release.TagName, "", fmt.Errorf("release found (%s) but no matching asset for %s/%s (candidates: %v)",
		release.TagName, targetOS, targetArch, names)
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip.
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

// performUpdate handles the download and extraction of the new version.
func performUpdate(downloadURL string) error {
	tmpFile, err := os.CreateTemp("", "swcat-update-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up
	tmpFile.Close()                 // close it, downloadFile will reopen it

	if err := downloadFile(downloadURL, tmpFile.Name()); err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	fmt.Println("Download complete. Unzipping...")
	// Unzip to current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	if err := unzip(tmpFile.Name(), cwd); err != nil {
		return fmt.Errorf("failed to unzip update: %w", err)
	}

	return nil
}

// findNewestLocalVersion scans the current directory for subdirectories matching "swcat-<version>"
// and returns the highest version found and its directory path.
// Returns "v0.0.0", "", nil if no versions are found.
func findNewestLocalVersion() (string, string, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return "", "", fmt.Errorf("failed to read directory: %w", err)
	}

	maxVersion := "v0.0.0"
	maxDir := ""

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Expect folder format: swcat-v1.2.3 or swcat-1.2.3
		if !strings.HasPrefix(name, "swcat-") {
			continue
		}

		verPart := strings.TrimPrefix(name, "swcat-")
		// Ensure it has 'v' prefix for semver
		if !strings.HasPrefix(verPart, "v") {
			verPart = "v" + verPart
		}

		if !semver.IsValid(verPart) {
			continue
		}

		if semver.Compare(verPart, maxVersion) > 0 {
			maxVersion = verPart
			maxDir = name
		}
	}

	return maxVersion, maxDir, nil
}

// launchApp executes the swcat binary found in the given directory.
func launchApp(dir string) error {
	exeName := "swcat"
	if runtime.GOOS == "windows" {
		exeName = "swcat.exe"
	}

	exePath := filepath.Join(dir, exeName)
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("executable not found at %s: %w", exePath, err)
	}

	// Prepare the command with all arguments passed to the launcher
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and wait for it to finish
	if err := cmd.Run(); err != nil {
		// If it's an ExitError, we might want to exit with the same code
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}

func main() {
	// 1. Get version of the actual app (scanning subfolders)
	currentVersion, _, err := findNewestLocalVersion()
	if err != nil {
		fmt.Printf("Warning: failed to scan for local versions: %v\n", err)
		currentVersion = "v0.0.0"
	}

	// 2. Check for update
	fmt.Printf("Checking for updates (current: %s)...\n", currentVersion)
	latestTag, downloadURL, err := getLatestRelease()

	if err != nil {
		// IMPORTANT: Log error but continue. Don't stop the user from working!
		fmt.Printf("Update check failed (%v). Launching local version...\n", err)
	} else if semver.Compare(latestTag, currentVersion) == 1 {
		fmt.Printf("New version %s found! Downloading from %s...\n", latestTag, downloadURL)

		if err := performUpdate(downloadURL); err != nil {
			fmt.Printf("Update failed: %v. Launching local version...\n", err)
		} else {
			fmt.Println("Update installed successfully.")
		}

	} else {
		fmt.Printf("Local version %s is up-to-date.\n", currentVersion)
	}

	// 3. Launch the newest app
	_, latestDir, err := findNewestLocalVersion()
	if err != nil || latestDir == "" {
		fmt.Printf("Fatal: no local version of swcat found to launch.\n")
		os.Exit(1)
	}

	if err := launchApp(latestDir); err != nil {
		fmt.Printf("Fatal: failed to launch swcat: %v\n", err)
		os.Exit(1)
	}
}
