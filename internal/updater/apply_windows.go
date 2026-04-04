//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func batchQuote(p string) string {
	return `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
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
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	installDir := filepath.Dir(exe)
	extractedRoot, err = filepath.Abs(extractedRoot)
	if err != nil {
		return err
	}

	mainExe := filepath.Join(installDir, "stock-sim.exe")
	if _, err := os.Stat(mainExe); err != nil {
		return fmt.Errorf("expected stock-sim.exe beside the running executable")
	}

	srcGlob := filepath.Join(extractedRoot, "*")
	destDir := installDir + string(os.PathSeparator)
	bat := fmt.Sprintf(
		"@echo off\r\ntimeout /t 2 /nobreak >nul\r\n"+
			"xcopy /E /Y /I %s %s\r\n"+
			"start \"\" %s\r\n"+
			"del \"%%~f0\"\r\n",
		batchQuote(srcGlob),
		batchQuote(destDir),
		batchQuote(mainExe),
	)

	batPath := filepath.Join(os.TempDir(), fmt.Sprintf("stock-sim-apply-%d.bat", os.Getpid()))
	if err := os.WriteFile(batPath, []byte(bat), 0600); err != nil {
		return err
	}

	// Detach from the GUI process so the batch keeps running after Wails exits (otherwise the
	// updater can be killed before timeout/xcopy completes).
	cmd := exec.Command("cmd.exe", "/C", "start", "/min", "stock-sim-update", "cmd.exe", "/C", batPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
