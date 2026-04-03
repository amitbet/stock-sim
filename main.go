package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"stock-sim/internal/httpapi"
)

func main() {
	defaultDBPath, searchRoots, found := discoverDefaultDBPath()
	dbPath := os.Getenv("SIM_DB_PATH")
	if dbPath == "" {
		if !found {
			log.Fatalf("no sqlite database found. Searched: %s. Put a .sqlite/.sqlite3/.db file next to the binary, in its parent directory, or in a sibling directory, or set SIM_DB_PATH.", strings.Join(searchRoots, ", "))
		}
		dbPath = defaultDBPath
	}

	cfg := httpapi.Config{
		Addr:       envOrDefault("SIM_ADDR", ":3002"),
		DBPath:     dbPath,
		UIDistPath: envOrDefault("SIM_UI_DIST", "internal/httpapi/dist"),
	}

	server, err := httpapi.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	url := browserURL(cfg.Addr)
	go openBrowserWhenReady(url)

	log.Printf("stock-sim listening on %s using db %s", cfg.Addr, cfg.DBPath)
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

var errStopWalk = fmt.Errorf("stop sqlite discovery")
