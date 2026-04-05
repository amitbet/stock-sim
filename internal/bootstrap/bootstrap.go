// Package bootstrap resolves SQLite paths, data sources, and shared env for server and Wails entrypoints.
package bootstrap

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
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const managedDBPrefix = "stock-sim-"

// DataConfig holds resolved DB path and default Yahoo/sqlite source after env + discovery.
type DataConfig struct {
	DBPath        string
	DefaultSource string
	SearchRoots   []string
}

// EnvOrDefault returns os.Getenv(key) or fallback when empty.
func EnvOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// LoadDataConfig applies SIM_DB_PATH, SIM_DATA_SOURCE, and filesystem discovery (same rules as the legacy main).
func LoadDataConfig() (DataConfig, error) {
	dataSource := strings.TrimSpace(os.Getenv("SIM_DATA_SOURCE"))
	configuredDBPath := strings.TrimSpace(os.Getenv("SIM_DB_PATH"))
	dbPath := configuredDBPath
	defaultDBPath, searchRoots, found := discoverDefaultDBPath()
	sourceDBPath := dbPath
	if sourceDBPath == "" && found {
		sourceDBPath = defaultDBPath
	}
	if sourceDBPath != "" {
		if isManagedSQLiteFile(sourceDBPath) {
			absPath, err := filepath.Abs(sourceDBPath)
			if err != nil {
				return DataConfig{}, err
			}
			// Normalization needs write access; skip if read-only/locked — NewStore still opens with mode=ro.
			if err := ensureDeleteJournalMode(absPath); err != nil {
				log.Printf("stock-sim: sqlite journal normalize skipped (%s): %v", absPath, err)
			}
			dbPath = absPath
		} else {
			var err error
			dbPath, err = prepareManagedDB(sourceDBPath)
			if err != nil {
				return DataConfig{}, err
			}
		}
	}

	if dataSource == "" {
		if dbPath != "" {
			dataSource = "sqlite"
		} else {
			dataSource = "yahoo"
		}
	}
	if dbPath != "" && !strings.EqualFold(dataSource, "sqlite") {
		// Prefer the local scanner DB whenever one is available; Yahoo remains selectable in the UI.
		dataSource = "sqlite"
	}
	if dbPath == "" && strings.EqualFold(dataSource, "sqlite") {
		return DataConfig{}, fmt.Errorf(
			"no sqlite database found. Searched: %s. Put a .sqlite/.sqlite3/.db file next to the binary, in its parent directory, or in a sibling directory, or set SIM_DB_PATH",
			strings.Join(searchRoots, ", "),
		)
	}

	return DataConfig{
		DBPath:        dbPath,
		DefaultSource: dataSource,
		SearchRoots:   searchRoots,
	}, nil
}

// BrowserURL converts a listen address like ":3002" or "0.0.0.0:3002" into a browser-openable http URL.
func BrowserURL(addr string) string {
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

// OpenBrowserWhenReady polls /api/health then opens the URL in the default browser.
func OpenBrowserWhenReady(baseURL string) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for attempt := 0; attempt < 50; attempt++ {
		resp, err := client.Get(baseURL + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if err := OpenBrowser(baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
	}
}

// OpenBrowser opens url in the platform default browser.
func OpenBrowser(url string) error {
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

	var wd string
	if currentWD, err := os.Getwd(); err == nil {
		wd = currentWD
	}
	searchRoots := defaultDBSearchRoots(exePath, wd)

	for _, root := range searchRoots {
		if path, ok := findSQLiteUnderRoot(root); ok {
			return path, searchRoots, true
		}
	}

	return "", searchRoots, false
}

// defaultDBSearchRoots lists directories to scan **one level deep only** (no recursion into
// subfolders). Order: exe directory + its parent + sibling folders; same for cwd when safe;
// for macOS .app bundles, also the folder that contains the .app.
func defaultDBSearchRoots(exePath, wd string) []string {
	var searchRoots []string

	binDir := filepath.Dir(exePath)
	collectShallowRoots(binDir, &searchRoots)

	if bundleParent, ok := macOSAppBundleParentDir(exePath); ok {
		// For packaged macOS apps, avoid walking the user's home-directory siblings.
		// Touching protected folders like Desktop/Documents/Downloads can trigger TCC
		// prompts during startup and delay backend readiness after auto-update relaunch.
		collectBundleParentRoots(bundleParent, &searchRoots)
	}

	// Skip cwd for packaged .app launches (Finder often uses "/" which would add "/" and all its
	// immediate subdirs as roots — still shallow, but noisy; bundle parent + exe chain already cover).
	if wd != "" && !isMacOSAppBundleExecutable(exePath) {
		collectShallowRoots(wd, &searchRoots)
	}

	return searchRoots
}

// collectBundleParentRoots adds the folder containing the .app and its direct child directories,
// but intentionally does not include the parent directory or its siblings.
func collectBundleParentRoots(anchor string, roots *[]string) {
	anchor = filepath.Clean(anchor)
	if anchor == "" || anchor == "." {
		return
	}
	appendUniqueSearchRoot(roots, anchor)

	entries, err := os.ReadDir(anchor)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		appendUniqueSearchRoot(roots, filepath.Join(anchor, e.Name()))
	}
}

// collectShallowRoots adds anchor, filepath.Dir(anchor), and each directory under that parent
// (sibling folders). Does nothing if anchor is empty or ".".
func collectShallowRoots(anchor string, roots *[]string) {
	anchor = filepath.Clean(anchor)
	if anchor == "" || anchor == "." {
		return
	}
	appendUniqueSearchRoot(roots, anchor)

	parent := filepath.Dir(anchor)
	if parent == "" || parent == anchor {
		return
	}
	appendUniqueSearchRoot(roots, parent)

	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		appendUniqueSearchRoot(roots, filepath.Join(parent, e.Name()))
	}
}

func isMacOSAppBundleExecutable(exePath string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(exePath))
	return strings.Contains(cleaned, ".app/Contents/MacOS/")
}

func macOSAppBundleParentDir(exePath string) (string, bool) {
	if !isMacOSAppBundleExecutable(exePath) {
		return "", false
	}
	bundleDir := filepath.Dir(filepath.Dir(filepath.Dir(exePath)))
	parentDir := filepath.Dir(bundleDir)
	if parentDir == "" || parentDir == bundleDir {
		return "", false
	}
	return parentDir, true
}

func appendUniqueSearchRoot(roots *[]string, dir string) {
	dir = filepath.Clean(dir)
	for _, existing := range *roots {
		if filepath.Clean(existing) == dir {
			return
		}
	}
	*roots = append(*roots, dir)
}

// findSQLiteUnderRoot looks only at **files directly in root** (no subdirectories). It prefers a
// non-managed .sqlite/.db with bars_daily; otherwise falls back to a managed stock-sim-*.sqlite.
func findSQLiteUnderRoot(root string) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var firstManaged string
	var found string
	for _, name := range names {
		path := filepath.Join(root, name)
		if !isSQLiteFile(path) {
			continue
		}
		if valid, err := sqliteHasBarsDaily(path); err != nil || !valid {
			continue
		}
		if isManagedSQLiteFile(path) {
			if firstManaged == "" {
				firstManaged = path
			}
			continue
		}
		found = path
		break
	}
	if found != "" {
		return found, true
	}
	if firstManaged != "" {
		return firstManaged, true
	}
	return "", false
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
			log.Printf("stock-sim: sqlite journal normalize skipped (%s): %v", managedPath, err)
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
		log.Printf("stock-sim: sqlite journal normalize skipped (%s): %v", managedPath, err)
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

func sqliteHasBarsDaily(path string) (bool, error) {
	dsn, err := sqliteDSN(path, "mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return false, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type IN ('table', 'view') AND lower(name) = 'bars_daily'
	`).Scan(&count); err != nil {
		return false, err
	}

	return count > 0, nil
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
