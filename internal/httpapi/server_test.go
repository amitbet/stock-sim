package httpapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

	t.Run("symbol info", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/symbol-info?symbol=QQQ")
		if err != nil {
			t.Fatalf("get symbol info: %v", err)
		}
		defer resp.Body.Close()

		var payload map[string]map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode symbol info: %v", err)
		}
		if payload["info"]["symbol"] != "QQQ" {
			t.Fatalf("unexpected symbol info payload: %+v", payload)
		}
	})

	t.Run("rejects out of range reference price override", func(t *testing.T) {
		body := `{
			"symbol":"QQQ",
			"reference_sell_date":"2024-01-02",
			"plan":"metadata:\n  name: Test\nreference_price: sell_price\nentry_rules:\n  - id: first-entry\n    label: First entry\n    trigger:\n      trading_days_since_reference: 1\n    action:\n      type: buy_percent\n      buy_percent: 20\nconstraints:\n  max_actions_per_day: 1\n  prevent_duplicate_level_buys: true\nexit: {}\n",
			"execution_price_mode":"same_day_close",
			"reference_price_mode":"close",
			"reference_price":410
		}`

		resp, err := http.Post(ts.URL+"/api/simulations/run", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("post simulation: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}

		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode simulation error: %v", err)
		}

		errorMessage, _ := payload["error"].(string)
		if !strings.Contains(errorMessage, "must be within the selected candle range") {
			t.Fatalf("expected range validation error, got %q", errorMessage)
		}
	})

	t.Run("simulation uses full future history for reclaim exit", func(t *testing.T) {
		body := `{
			"symbol":"QQQ",
			"reference_sell_date":"2024-01-02",
			"plan":"metadata:\n  name: Test\nreference_price: sell_price\nentry_rules:\n  - id: first-entry\n    label: First entry\n    trigger:\n      trading_days_since_reference: 1\n    action:\n      type: buy_percent\n      buy_percent: 100\nconstraints:\n  max_actions_per_day: 1\n  prevent_duplicate_level_buys: true\nexit: {}\n",
			"execution_price_mode":"exact",
			"reference_price_mode":"close"
		}`

		resp, err := http.Post(ts.URL+"/api/simulations/run", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("post simulation: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode simulation: %v", err)
		}

		summary, _ := payload["summary"].(map[string]any)
		if got := summary["end_date"]; got != "2024-01-10" {
			t.Fatalf("expected end_date 2024-01-10, got %v", got)
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
			('QQQ', '2024-01-03', 401, 402, 395, 399, 1100000, 399.5),
			('QQQ', '2024-01-04', 399, 401, 394, 400, 1200000, 399.5),
			('QQQ', '2024-01-10', 401, 410, 400, 409, 1150000, 408),
			('SPY', '2024-01-02', 470, 472, 468, 471, 900000, 470.5)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed sqlite: %v", err)
		}
	}

	return dbPath
}
