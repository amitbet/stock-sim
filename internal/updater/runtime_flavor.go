package updater

import (
	"os"
	"path/filepath"
	"strings"
)

func CurrentExecutableName() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Base(exe)
}

func useLegacyWindows7Asset(exeName string) bool {
	if isWindows7OrEarlier() {
		return true
	}
	return isLegacyWindows7Executable(exeName)
}

func isLegacyWindows7Executable(exeName string) bool {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(exeName)))
	return strings.Contains(name, "win7")
}
