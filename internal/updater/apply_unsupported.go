//go:build !darwin && !windows

package updater

import "fmt"

func applyPlatform(_ string) error {
	return fmt.Errorf("auto-update is only supported on macOS and Windows")
}
