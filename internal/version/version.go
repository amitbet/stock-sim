// Package version holds the application version (set via -ldflags at build time).
package version

// Version is the release version (e.g. 1.2.3). Wails and CI should set via -ldflags "-X stock-sim/internal/version.Version=..."
var Version = "dev"

// UpdateRepo is "owner/repo" for GitHub release API (update checks). Default is the public home repo;
// CI may override via -ldflags "-X stock-sim/internal/version.UpdateRepo=...", or use env STOCK_SIM_UPDATE_REPO.
var UpdateRepo = "amitbet/stock-sim"
