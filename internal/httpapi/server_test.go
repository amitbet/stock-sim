package httpapi_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"stock-sim/internal/httpapi"

	_ "modernc.org/sqlite"
)

func TestAPIEndpoints(t *testing.T) {
	dbPath := createTestDB(t)
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

func createTestDB(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "scanner.sqlite")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	statements := []string{
		`CREATE TABLE bars_daily (
			symbol TEXT NOT NULL,
			date TEXT NOT NULL,
			open REAL NOT NULL,
			high REAL NOT NULL,
			low REAL NOT NULL,
			close REAL NOT NULL,
			volume REAL NOT NULL,
			vwap REAL
		)`,
		`INSERT INTO bars_daily (symbol, date, open, high, low, close, volume, vwap) VALUES
			('QQQ', '2024-01-02', 400, 405, 398, 403, 1000000, 402),
			('QQQ', '2024-01-03', 403, 406, 401, 404, 1100000, 403.5),
			('QQQ', '2024-01-04', 404, 408, 402, 407, 1200000, 405.5),
			('SPY', '2024-01-02', 470, 472, 468, 471, 900000, 470.5)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed sqlite: %v", err)
		}
	}

	return dbPath
}
