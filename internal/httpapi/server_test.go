package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"stock-sim/internal/httpapi"
)

func TestAPIEndpoints(t *testing.T) {
	dbPath := locateDBPath(t)
	server, err := httpapi.NewServer(httpapi.Config{
		Addr:       ":0",
		DBPath:     dbPath,
		UIDistPath: filepath.Join("dist"),
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	t.Run("symbols", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/symbols")
		if err != nil {
			t.Fatalf("get symbols: %v", err)
		}
		defer resp.Body.Close()
		var payload map[string][]string
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode symbols: %v", err)
		}
		if len(payload["symbols"]) == 0 {
			t.Fatal("expected symbols")
		}
	})

	t.Run("bars", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/bars?symbol=QQQ&from=2024-01-01&to=2024-02-01")
		if err != nil {
			t.Fatalf("get bars: %v", err)
		}
		defer resp.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode bars: %v", err)
		}
		if payload["error"] != nil {
			t.Fatalf("unexpected bars error: %v", payload["error"])
		}
		bars, ok := payload["bars"].([]any)
		if !ok || len(bars) == 0 {
			t.Fatal("expected bars")
		}
	})
}

func locateDBPath(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	candidates := []string{
		filepath.Join(wd, "..", "..", "stock-scanner", "data", "scanner.sqlite"),
		filepath.Join(wd, "..", "..", "..", "stock-scanner", "data", "scanner.sqlite"),
		filepath.Join(wd, "..", "..", "..", "..", "stock-scanner", "data", "scanner.sqlite"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	t.Fatalf("could not locate scanner sqlite from %s; tried %v", wd, candidates)
	return ""
}
