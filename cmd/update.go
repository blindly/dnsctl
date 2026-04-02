package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update dnsctl to the latest version",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	const repo = "blindly/dnsctl"

	fmt.Println("Checking for updates...")

	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return fmt.Errorf("checking latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API returned %d — no releases found?", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("parsing release info: %w", err)
	}

	// Find the right asset for this OS/arch
	target := fmt.Sprintf("dnsctl-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == target {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Get current binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding current binary: %w", err)
	}

	// Check if already on latest
	if strings.Contains(release.TagName, Version) && Version != "dev" {
		fmt.Printf("Already on latest version (%s).\n", release.TagName)
		return nil
	}

	fmt.Printf("Downloading %s...\n", release.TagName)

	dlResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", dlResp.StatusCode)
	}

	// Write to temp file next to the binary
	tmpFile, err := os.CreateTemp("", "dnsctl-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing binary: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}

	// Replace current binary
	if err := os.Rename(tmpFile.Name(), execPath); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	fmt.Printf("Updated to %s.\n", release.TagName)
	return nil
}
