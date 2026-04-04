package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"stock-sim/internal/bootstrap"
	"stock-sim/internal/httpapi"
	"stock-sim/internal/updater"
	"stock-sim/internal/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails backend: REST API on loopback and bindings for the webview.
type App struct {
	ctx    context.Context
	server *httpapi.Server
	apiURL string
}

// NewApp creates the application struct used by Wails.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	data, err := bootstrap.LoadDataConfig()
	if err != nil {
		log.Fatal(err)
	}

	addr := bootstrap.EnvOrDefault("SIM_ADDR", "127.0.0.1:0")
	cfg := httpapi.Config{
		Addr:          addr,
		DBPath:        data.DBPath,
		DefaultSource: data.DefaultSource,
		UIDistPath:    bootstrap.EnvOrDefault("SIM_UI_DIST", "internal/httpapi/dist"),
		APIOnly:       true,
	}

	srv, err := httpapi.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	a.server = srv

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	for i := 0; i < 200; i++ {
		if u := srv.HTTPBaseURL(); u != "" {
			a.apiURL = u
			log.Printf("stock-sim API %s (data source %s)", u, data.DBPath)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	log.Fatal("API server did not become ready")
}

func (a *App) shutdown(ctx context.Context) {
	if a.server != nil {
		_ = a.server.Shutdown(ctx)
	}
}

// GetAPIBaseURL returns the http://127.0.0.1:port origin for REST calls from the webview.
func (a *App) GetAPIBaseURL() string {
	return a.apiURL
}

// Version returns the application version string.
func (a *App) Version() string {
	return version.Version
}

// CheckForUpdates compares the running version to the latest GitHub release (optional repo via env).
func (a *App) CheckForUpdates() (*updater.Status, error) {
	return updater.Check(version.Version)
}

// ApplyUpdateAndRestart downloads the latest Wails zip for this OS/arch from GitHub, replaces the install, and quits.
// Set STOCK_SIM_UPDATE_REPO=owner/repo (or GITHUB_REPOSITORY). Requires a semver release build (not "dev").
func (a *App) ApplyUpdateAndRestart() error {
	if a.ctx == nil {
		return errors.New("app not ready")
	}
	if err := updater.Apply(version.Version); err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}
