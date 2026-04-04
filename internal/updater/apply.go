package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Apply downloads the latest Wails release zip for this OS/arch, extracts it, and schedules a
// replace + relaunch. The process should exit shortly after Apply returns (caller calls runtime.Quit).
func Apply(currentVersion string) error {
	if currentVersion == "" || currentVersion == "dev" {
		return fmt.Errorf("auto-update requires a release build version (not %q)", currentVersion)
	}
	if _, err := repoFromEnv(); err != nil {
		return err
	}

	rel, err := FetchLatestRelease()
	if err != nil {
		return err
	}

	newer, err := CompareVersions(currentVersion, rel.TagName)
	if err != nil {
		return err
	}
	if !newer {
		return fmt.Errorf("already up to date (%s)", currentVersion)
	}

	asset, err := PickWailsZipAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if asset.BrowserDownloadURL == "" {
		return fmt.Errorf("release asset %q has no download URL", asset.Name)
	}

	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("stock-sim-update-%d.zip", time.Now().UnixNano()))
	if err := downloadFile(asset.BrowserDownloadURL, zipPath); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(zipPath)

	extractDir := filepath.Join(os.TempDir(), fmt.Sprintf("stock-sim-update-extract-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}
	// Do not remove extractDir here: macOS/Windows apply scripts copy from it after the app exits.

	if err := unzip(zipPath, extractDir); err != nil {
		_ = os.RemoveAll(extractDir)
		return fmt.Errorf("extract: %w", err)
	}

	return applyPlatform(extractDir)
}
