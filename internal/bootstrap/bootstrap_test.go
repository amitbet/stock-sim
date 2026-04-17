package bootstrap

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFindSQLiteUnderRootPrefersBarsDaily(t *testing.T) {
	root := t.TempDir()

	invalidPath := filepath.Join(root, "scanner.sqlite")
	createSQLiteFile(t, invalidPath, `CREATE TABLE settings (id INTEGER PRIMARY KEY);`)

	validPath := filepath.Join(root, "scanner-valid.sqlite")
	createSQLiteFile(t, validPath, `CREATE TABLE bars_daily (symbol TEXT, date TEXT, open REAL, high REAL, low REAL, close REAL, volume REAL, vwap REAL);`)

	got, ok := findSQLiteUnderRoot(root)
	if !ok {
		t.Fatal("expected sqlite file to be discovered")
	}
	if got != validPath {
		t.Fatalf("expected valid sqlite %q, got %q", validPath, got)
	}
}

func TestFindSQLiteUnderRootSkipsInvalidSQLiteFiles(t *testing.T) {
	root := t.TempDir()

	invalidPath := filepath.Join(root, "scanner.sqlite")
	createSQLiteFile(t, invalidPath, `CREATE TABLE settings (id INTEGER PRIMARY KEY);`)

	got, ok := findSQLiteUnderRoot(root)
	if ok {
		t.Fatalf("expected invalid sqlite to be skipped, got %q", got)
	}
}

func TestSQLiteHasBarsDaily(t *testing.T) {
	root := t.TempDir()
	validPath := filepath.Join(root, "scanner.sqlite")
	createSQLiteFile(t, validPath, `CREATE TABLE bars_daily (symbol TEXT, date TEXT, open REAL, high REAL, low REAL, close REAL, volume REAL, vwap REAL);`)

	hasBarsDaily, err := sqliteHasBarsDaily(validPath)
	if err != nil {
		t.Fatalf("sqliteHasBarsDaily returned error: %v", err)
	}
	if !hasBarsDaily {
		t.Fatal("expected bars_daily table to be detected")
	}
}

func TestDefaultDBSearchRootsSkipsCWDForMacOSAppBundle(t *testing.T) {
	root := t.TempDir()
	exePath := filepath.Join(root, "stock-sim.app", "Contents", "MacOS", "stock-sim")
	wd := string(filepath.Separator)

	searchRoots := defaultDBSearchRoots(exePath, wd)

	if !containsString(searchRoots, root) {
		t.Fatalf("expected app bundle parent %q to be included for app bundle launch, got %v", root, searchRoots)
	}
	if containsString(searchRoots, wd) {
		t.Fatalf("expected cwd %q to be skipped for app bundle launch, got %v", wd, searchRoots)
	}
	if containsString(searchRoots, filepath.Join(wd, "bin")) {
		t.Fatalf("expected cwd bin %q to be skipped for app bundle launch, got %v", filepath.Join(wd, "bin"), searchRoots)
	}
}

func TestDefaultDBSearchRootsIncludesCWDForCLI(t *testing.T) {
	root := t.TempDir()
	exePath := filepath.Join(root, "build", "bin", "stock-sim")
	wd := filepath.Join(root, "workspace")

	searchRoots := defaultDBSearchRoots(exePath, wd)

	if !containsString(searchRoots, wd) {
		t.Fatalf("expected cwd %q to be included for CLI launch, got %v", wd, searchRoots)
	}
}

func TestLoadDataConfigDefaultsToYahooWhenDBExists(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "scanner.sqlite")
	createSQLiteFile(t, dbPath, `CREATE TABLE bars_daily (symbol TEXT, date TEXT, open REAL, high REAL, low REAL, close REAL, volume REAL, vwap REAL);`)

	originalDBPath := os.Getenv("SIM_DB_PATH")
	originalDataSource := os.Getenv("SIM_DATA_SOURCE")
	t.Cleanup(func() {
		_ = os.Setenv("SIM_DB_PATH", originalDBPath)
		if originalDataSource == "" {
			_ = os.Unsetenv("SIM_DATA_SOURCE")
			return
		}
		_ = os.Setenv("SIM_DATA_SOURCE", originalDataSource)
	})

	if err := os.Setenv("SIM_DB_PATH", dbPath); err != nil {
		t.Fatalf("set SIM_DB_PATH: %v", err)
	}
	_ = os.Unsetenv("SIM_DATA_SOURCE")

	cfg, err := LoadDataConfig()
	if err != nil {
		t.Fatalf("LoadDataConfig returned error: %v", err)
	}
	if cfg.DefaultSource != "yahoo" {
		t.Fatalf("expected default source yahoo, got %q", cfg.DefaultSource)
	}
}

func TestLoadDataConfigRequiresExplicitSQLitePath(t *testing.T) {
	originalDBPath := os.Getenv("SIM_DB_PATH")
	originalDataSource := os.Getenv("SIM_DATA_SOURCE")
	t.Cleanup(func() {
		_ = os.Setenv("SIM_DB_PATH", originalDBPath)
		if originalDataSource == "" {
			_ = os.Unsetenv("SIM_DATA_SOURCE")
			return
		}
		_ = os.Setenv("SIM_DATA_SOURCE", originalDataSource)
	})

	_ = os.Unsetenv("SIM_DB_PATH")
	if err := os.Setenv("SIM_DATA_SOURCE", "sqlite"); err != nil {
		t.Fatalf("set SIM_DATA_SOURCE: %v", err)
	}

	_, err := LoadDataConfig()
	if err == nil {
		t.Fatal("expected sqlite startup without SIM_DB_PATH to fail")
	}
	if !strings.Contains(err.Error(), "SIM_DB_PATH") {
		t.Fatalf("expected SIM_DB_PATH guidance, got %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func createSQLiteFile(t *testing.T, path string, schema string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite %s: %v", path, err)
	}
	defer db.Close()

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema in %s: %v", path, err)
	}
}
