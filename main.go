package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"stock-sim/internal/httpapi"

	_ "modernc.org/sqlite"
)

const managedDBPrefix = "stock-sim-"

func main() {
	dataSource := strings.TrimSpace(os.Getenv("SIM_DATA_SOURCE"))
	dbPath := os.Getenv("SIM_DB_PATH")
	defaultDBPath, searchRoots, found := discoverDefaultDBPath()
	sourceDBPath := dbPath
	if sourceDBPath == "" && found {
		sourceDBPath = defaultDBPath
	}
	if sourceDBPath != "" {
		var err error
		dbPath, err = prepareManagedDB(sourceDBPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	if dataSource == "" {
		if dbPath != "" {
			dataSource = "sqlite"
		} else {
			dataSource = "yahoo"
		}
	}
	if dbPath == "" && strings.EqualFold(dataSource, "sqlite") {
		log.Fatalf("no sqlite database found. Searched: %s. Put a .sqlite/.sqlite3/.db file next to the binary, in its parent directory, or in a sibling directory, or set SIM_DB_PATH.", strings.Join(searchRoots, ", "))
	}

	cfg := httpapi.Config{
		Addr:          envOrDefault("SIM_ADDR", ":3002"),
		DBPath:        dbPath,
		DefaultSource: dataSource,
		UIDistPath:    envOrDefault("SIM_UI_DIST", "internal/httpapi/dist"),
	}

	server, err := httpapi.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	url := browserURL(cfg.Addr)
	go openBrowserWhenReady(url)

	log.Printf("stock-sim listening on %s using data source %s", cfg.Addr, cfg.DBPath)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func browserURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if _, portOnly, splitErr := net.SplitHostPort("127.0.0.1" + addr); splitErr == nil {
			return fmt.Sprintf("http://127.0.0.1:%s", portOnly)
		}
		return "http://127.0.0.1:3002"
	}

	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}

	return fmt.Sprintf("http://%s:%s", host, port)
}

func openBrowserWhenReady(url string) {
	client := &http.Client{Timeout: 500 * time.Millisecond}

	for attempt := 0; attempt < 50; attempt++ {
		resp, err := client.Get(url + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	if err := openBrowser(url); err != nil {
		log.Printf("open browser: %v", err)
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func discoverDefaultDBPath() (string, []string, bool) {
	exePath, err := os.Executable()
	if err != nil {
		return "", nil, false
	}

	binDir := filepath.Dir(exePath)
	parentDir := filepath.Dir(binDir)

	searchRoots := []string{binDir, parentDir}

	entries, err := os.ReadDir(parentDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && filepath.Join(parentDir, entry.Name()) != binDir {
				searchRoots = append(searchRoots, filepath.Join(parentDir, entry.Name()))
			}
		}
	}

	for _, root := range searchRoots {
		if path, ok := findSQLiteUnderRoot(root); ok {
			return path, searchRoots, true
		}
	}

	return "", searchRoots, false
}

func findSQLiteUnderRoot(root string) (string, bool) {
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == "node_modules" || name == ".git" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if isManagedSQLiteFile(path) {
			return nil
		}
		if isSQLiteFile(path) {
			found = path
			return errStopWalk
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return "", false
	}
	if found == "" {
		return "", false
	}
	return found, true
}

func isSQLiteFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".sqlite") ||
		strings.HasSuffix(lower, ".sqlite3") ||
		strings.HasSuffix(lower, ".db")
}

func prepareManagedDB(sourcePath string) (string, error) {
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("resolve source sqlite path: %w", err)
	}

	managedPath, err := managedDBPath(sourcePath)
	if err != nil {
		return "", err
	}

	if samePath(sourcePath, managedPath) {
		if err := ensureDeleteJournalMode(managedPath); err != nil {
			return "", err
		}
		return managedPath, nil
	}

	refresh, err := managedCopyNeedsRefresh(sourcePath, managedPath)
	if err != nil {
		return "", err
	}
	if refresh {
		if err := stageSQLiteCopy(sourcePath, managedPath); err != nil {
			return "", err
		}
	}

	if err := ensureDeleteJournalMode(managedPath); err != nil {
		return "", err
	}

	return managedPath, nil
}

func managedDBPath(sourcePath string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("determine executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, managedSQLiteName(filepath.Base(sourcePath))), nil
}

func managedCopyNeedsRefresh(sourcePath, managedPath string) (bool, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return false, fmt.Errorf("stat source sqlite %s: %w", sourcePath, err)
	}

	managedInfo, err := os.Stat(managedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("stat staged sqlite %s: %w", managedPath, err)
	}

	if sourceInfo.Size() != managedInfo.Size() {
		return true, nil
	}
	if managedInfo.ModTime().Before(sourceInfo.ModTime()) {
		return true, nil
	}

	return false, nil
}

func stageSQLiteCopy(sourcePath, managedPath string) error {
	tempPath := managedPath + ".tmp"
	for _, path := range sqliteSidecars(tempPath) {
		_ = os.Remove(path)
	}

	if err := copyFile(sourcePath, tempPath); err != nil {
		return fmt.Errorf("copy sqlite to staging file: %w", err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		sourceSidecar := sourcePath + suffix
		tempSidecar := tempPath + suffix
		if _, err := os.Stat(sourceSidecar); err == nil {
			if err := copyFile(sourceSidecar, tempSidecar); err != nil {
				return fmt.Errorf("copy sqlite sidecar %s: %w", suffix, err)
			}
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat sqlite sidecar %s: %w", sourceSidecar, err)
		}
		_ = os.Remove(tempSidecar)
	}

	for _, path := range sqliteSidecars(managedPath) {
		_ = os.Remove(path)
	}

	if err := os.Rename(tempPath, managedPath); err != nil {
		return fmt.Errorf("replace staged sqlite: %w", err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		tempSidecar := tempPath + suffix
		managedSidecar := managedPath + suffix
		if _, err := os.Stat(tempSidecar); err == nil {
			if err := os.Rename(tempSidecar, managedSidecar); err != nil {
				return fmt.Errorf("replace staged sqlite sidecar %s: %w", suffix, err)
			}
		}
	}

	return nil
}

func copyFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", sourcePath, err)
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat source file %s: %w", sourcePath, err)
	}

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return fmt.Errorf("open target file %s: %w", targetPath, err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("copy data to %s: %w", targetPath, err)
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync target file %s: %w", targetPath, err)
	}
	if err := os.Chtimes(targetPath, time.Now(), info.ModTime()); err != nil {
		return fmt.Errorf("apply source timestamps to %s: %w", targetPath, err)
	}

	return nil
}

func ensureDeleteJournalMode(path string) error {
	dsn, err := sqliteDSN(path, "")
	if err != nil {
		return err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open staged sqlite for journal normalization: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		return fmt.Errorf("checkpoint staged sqlite wal: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=DELETE;"); err != nil {
		return fmt.Errorf("set staged sqlite journal mode to DELETE: %w", err)
	}

	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		_ = os.Remove(sidecar)
	}

	return nil
}

func sqliteDSN(path, rawQuery string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve sqlite dsn path: %w", err)
	}

	dsnPath := filepath.ToSlash(absPath)
	if filepath.VolumeName(absPath) != "" && !strings.HasPrefix(dsnPath, "/") {
		dsnPath = "/" + dsnPath
	}

	dsn := url.URL{
		Scheme:   "file",
		Path:     dsnPath,
		RawQuery: rawQuery,
	}

	return dsn.String(), nil
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func sqliteSidecars(path string) []string {
	return []string{path, path + "-wal", path + "-shm"}
}

func managedSQLiteName(base string) string {
	return managedDBPrefix + base
}

func isManagedSQLiteFile(path string) bool {
	return strings.HasPrefix(strings.ToLower(filepath.Base(path)), strings.ToLower(managedDBPrefix)) && isSQLiteFile(path)
}

var errStopWalk = fmt.Errorf("stop sqlite discovery")
