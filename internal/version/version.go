// Package version holds the application version (set via -ldflags at build time).
package version

// Version is the release version (e.g. 1.2.3). Wails and CI should set via -ldflags "-X stock-sim/internal/version.Version=..."
var Version = "dev"
