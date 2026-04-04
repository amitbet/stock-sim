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
sleep 2
DEST=%q
NEW=%q
NAME="stock-sim.app"
rm -rf "$DEST/$NAME.old"
if [ -d "$DEST/$NAME" ]; then
  mv "$DEST/$NAME" "$DEST/$NAME.old"
fi
cp -R "$NEW" "$DEST/"
open "$DEST/$NAME" || true
rm -rf "$DEST/$NAME.old"
rm -f "$0"
`, destParent, newApp)

	shPath := filepath.Join(os.TempDir(), fmt.Sprintf("stock-sim-apply-%d.sh", os.Getpid()))
	if err := os.WriteFile(shPath, []byte(script), 0700); err != nil {
		return err
	}

	cmd := exec.Command("/bin/bash", shPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
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
