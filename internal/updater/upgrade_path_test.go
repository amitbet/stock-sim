package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUpgradePath_105_to_106 documents the expected semver and asset-selection path for moving
// from a 1.0.5 build to a GitHub latest of v1.0.6 (no network; no real download/apply).
func TestUpgradePath_105_to_106(t *testing.T) {
	t.Run("CompareVersions", func(t *testing.T) {
		ok, err := CompareVersions("1.0.5", "v1.0.6")
		if err != nil {
			t.Fatalf("CompareVersions: %v", err)
		}
		if !ok {
			t.Fatal("expected v1.0.6 > 1.0.5")
		}
		ok, err = CompareVersions("1.0.5", "1.0.6")
		if err != nil {
			t.Fatalf("CompareVersions bare tag: %v", err)
		}
		if !ok {
			t.Fatal("expected 1.0.6 > 1.0.5")
		}
	})

	t.Run("computeUpdateStatus", func(t *testing.T) {
		st := computeUpdateStatus("1.0.5", &Release{TagName: "v1.0.6"})
		if !st.UpdateAvailable {
			t.Fatalf("UpdateAvailable: got false, want true (%s)", st.Message)
		}
		if st.Latest != "v1.0.6" {
			t.Fatalf("Latest: got %q", st.Latest)
		}
		if st.Current != "1.0.5" {
			t.Fatalf("Current: got %q", st.Current)
		}
	})

	t.Run("Check_with_mock_GitHub_API", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/test-owner/test-repo/releases/latest" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v1.0.6",
				Assets: []ReleaseAsset{
					{Name: "stock-sim-darwin-arm64-v1.0.6.zip", BrowserDownloadURL: "https://example.invalid/asset.zip"},
				},
			})
		}))
		t.Cleanup(srv.Close)

		prev := githubAPIRoot
		githubAPIRoot = srv.URL
		t.Cleanup(func() { githubAPIRoot = prev })

		t.Setenv("STOCK_SIM_UPDATE_REPO", "test-owner/test-repo")

		st, err := Check("1.0.5")
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if !st.UpdateAvailable {
			t.Fatalf("UpdateAvailable: got false (%s)", st.Message)
		}
		if st.Latest != "v1.0.6" {
			t.Fatalf("Latest: got %q", st.Latest)
		}
	})

	t.Run("Apply_prechecks_pick_zip_per_desktop_platform", func(t *testing.T) {
		rel := &Release{
			TagName: "v1.0.6",
			Assets: []ReleaseAsset{
				{Name: "stock-sim-windows7-amd64-html-v1.0.6.zip", BrowserDownloadURL: "https://x/w7.zip"},
				{Name: "stock-sim-darwin-arm64-v1.0.6.zip", BrowserDownloadURL: "https://x/mac.zip"},
				{Name: "stock-sim-darwin-amd64-v1.0.6.zip", BrowserDownloadURL: "https://x/macintel.zip"},
				{Name: "stock-sim-windows-amd64-v1.0.6.zip", BrowserDownloadURL: "https://x/win.zip"},
			},
		}
		newer, err := CompareVersions("1.0.5", rel.TagName)
		if err != nil || !newer {
			t.Fatalf("CompareVersions: newer=%v err=%v", newer, err)
		}
		cases := []struct {
			goos, goarch, wantURL string
		}{
			{"darwin", "arm64", "https://x/mac.zip"},
			{"darwin", "amd64", "https://x/macintel.zip"},
			{"windows", "amd64", "https://x/win.zip"},
		}
		for _, tc := range cases {
			asset, err := PickWailsZipAsset(rel.Assets, tc.goos, tc.goarch)
			if err != nil {
				t.Fatalf("PickWailsZipAsset(%s/%s): %v", tc.goos, tc.goarch, err)
			}
			if asset.BrowserDownloadURL != tc.wantURL {
				t.Fatalf("%s/%s: got %q want %q", tc.goos, tc.goarch, asset.BrowserDownloadURL, tc.wantURL)
			}
		}
	})

	t.Run("Apply_prechecks_pick_zip_for_win7_legacy_bundle", func(t *testing.T) {
		rel := &Release{
			TagName: "v1.0.6",
			Assets: []ReleaseAsset{
				{Name: "stock-sim-windows7-amd64-html-v1.0.6.zip", BrowserDownloadURL: "https://x/w7.zip"},
				{Name: "stock-sim-windows-amd64-v1.0.6.zip", BrowserDownloadURL: "https://x/win.zip"},
			},
		}
		asset, err := PickReleaseZipAsset(rel.Assets, "windows", "amd64", "stock-sim-win7.exe")
		if err != nil {
			t.Fatalf("PickReleaseZipAsset(win7): %v", err)
		}
		if asset.BrowserDownloadURL != "https://x/w7.zip" {
			t.Fatalf("win7: got %q want %q", asset.BrowserDownloadURL, "https://x/w7.zip")
		}
	})
}
