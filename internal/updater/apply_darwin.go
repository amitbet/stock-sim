//go:build darwin

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// preflightApplyInstallLocation fails early when in-place replace cannot work (e.g. App Translocation
// from opening the app from Downloads, or running from a .dmg).
func preflightApplyInstallLocation() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	bundle, err := findParentAppBundle(filepath.Dir(exe))
	if err != nil {
		return fmt.Errorf("auto-update only works when running from a .app bundle: %w", err)
	}
	bundle, err = filepath.EvalSymlinks(bundle)
	if err != nil {
		return err
	}
	return darwinUpdateBlockedReason(bundle)
}

func darwinUpdateBlockedReason(bundle string) error {
	lower := strings.ToLower(bundle)
	if strings.Contains(lower, "apptranslocation") {
		return fmt.Errorf(
			`macOS is running this app from a temporary location (App Translocation), which is read-only — common when the app was first opened from Downloads. Quit, move "stock-sim.app" to Applications or ~/Applications, open it once, then try Update again`,
		)
	}
	if strings.HasPrefix(bundle, "/Volumes/") {
		return fmt.Errorf(
			"auto-update cannot replace an app running from a disk image. Quit, drag stock-sim.app to Applications, eject the image, open the copied app, then try Update again",
		)
	}
	return nil
}

func applyPlatform(extractedRoot string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	bundle, err := findParentAppBundle(filepath.Dir(exe))
	if err != nil {
		return fmt.Errorf("auto-update only works when running from a .app bundle: %w", err)
	}
	if err := darwinUpdateBlockedReason(bundle); err != nil {
		return err
	}

	newApp, err := findStockSimApp(extractedRoot)
	if err != nil {
		return err
	}
	newApp, err = filepath.Abs(newApp)
	if err != nil {
		return err
	}

	destParent, err := filepath.Abs(filepath.Dir(bundle))
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
DEST=%q
NEW=%q
NAME="stock-sim.app"
echo "stock-sim apply: waiting 2s then DEST=$DEST NEW=$NEW"
sleep 2
echo "stock-sim apply: replacing bundle"
rm -rf "$DEST/$NAME.old"
if [ -d "$DEST/$NAME" ]; then
  mv "$DEST/$NAME" "$DEST/$NAME.old"
fi
cp -R "$NEW" "$DEST/"
echo "stock-sim apply: launching"
open "$DEST/$NAME" || true
rm -rf "$DEST/$NAME.old"
rm -f "$0"
echo "stock-sim apply: done"
`, destParent, newApp)

	shPath := filepath.Join(os.TempDir(), fmt.Sprintf("stock-sim-apply-%d.sh", os.Getpid()))
	if err := os.WriteFile(shPath, []byte(script), 0700); err != nil {
		return err
	}

	logPath := darwinUpdateLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		logPath = filepath.Join(os.TempDir(), "stock-sim-update.log")
	}
	// Short-lived bash -c starts nohup then exits so the updater survives runtime.Quit (Wails child teardown).
	launcher := fmt.Sprintf(`echo "[$(date)] stock-sim update starting" >>%q; nohup /bin/bash %q >>%q 2>&1 </dev/null &`, logPath, shPath, logPath)
	cmd := exec.Command("/bin/bash", "-c", launcher)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start apply launcher: %w", err)
	}
	return nil
}

func darwinUpdateLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "stock-sim-update.log")
	}
	return filepath.Join(home, "Library", "Logs", "stock-sim-update.log")
}

func findParentAppBundle(dir string) (string, error) {
	for {
		base := filepath.Base(dir)
		if strings.HasSuffix(base, ".app") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .app in path")
		}
		dir = parent
	}
}

func findStockSimApp(root string) (string, error) {
	direct := filepath.Join(root, "stock-sim.app")
	if st, err := os.Stat(direct); err == nil && st.IsDir() {
		return direct, nil
	}
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".app") && filepath.Base(path) == "stock-sim.app" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("stock-sim.app not found inside update zip")
	}
	return found, nil
}
