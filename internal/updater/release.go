package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Release holds GitHub GET /releases/latest payload fields we need.
type Release struct {
	TagName string        `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

// ReleaseAsset is one downloadable file on a release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func repoFromEnv() (string, error) {
	repo := strings.TrimSpace(os.Getenv("STOCK_SIM_UPDATE_REPO"))
	if repo == "" {
		repo = strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY"))
	}
	if repo == "" {
		return "", fmt.Errorf("set STOCK_SIM_UPDATE_REPO=owner/repo (or run under GITHUB_REPOSITORY) for updates")
	}
	return repo, nil
}

func authHeader() string {
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		return "Bearer " + tok
	}
	return ""
}

// FetchLatestRelease returns the latest published release (same as /releases/latest).
func FetchLatestRelease() (*Release, error) {
	repo, err := repoFromEnv()
	if err != nil {
		return nil, err
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if h := authHeader(); h != "" {
		req.Header.Set("Authorization", h)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// PickWailsZipAsset finds the portable Wails zip for this OS/arch (matches CI release asset names).
// Names look like stock-sim-darwin-arm64-v1.0.0.zip (older releases used ...-darwin-arm64-wails-...).
func PickWailsZipAsset(assets []ReleaseAsset, goos, goarch string) (*ReleaseAsset, error) {
	var needle string
	switch {
	case goos == "darwin" && goarch == "arm64":
		needle = "darwin-arm64"
	case goos == "darwin" && goarch == "amd64":
		needle = "darwin-amd64"
	case goos == "windows" && goarch == "amd64":
		needle = "windows-amd64"
	default:
		return nil, fmt.Errorf("auto-update not supported on %s/%s (only darwin/arm64, darwin/amd64, windows/amd64)", goos, goarch)
	}
	needle = strings.ToLower(needle)
	for i := range assets {
		a := &assets[i]
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".zip") {
			continue
		}
		if strings.Contains(name, "windows7-amd64-html") {
			continue
		}
		// Win7 browser bundle is windows7-amd64-*; substring "windows-amd64" does not match it, but
		// exclude explicitly in case naming changes.
		if needle == "windows-amd64" && strings.Contains(name, "windows7") {
			continue
		}
		if strings.Contains(name, needle) {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no desktop zip asset containing %q in this release", needle)
}

// CompareVersions returns true if latestTag is newer than current (semver).
func CompareVersions(current, latestTag string) (bool, error) {
	if latestTag == "" || current == "" || current == "dev" {
		return false, nil
	}
	c := normalizeSemver(current)
	l := normalizeSemver(latestTag)
	if c == "" || l == "" {
		return false, fmt.Errorf("non-semver version(s): current=%q latest=%q", current, latestTag)
	}
	return semver.Compare(l, c) > 0, nil
}

func normalizeSemver(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return ""
	}
	if !semver.IsValid("v" + v) {
		return ""
	}
	return "v" + v
}
