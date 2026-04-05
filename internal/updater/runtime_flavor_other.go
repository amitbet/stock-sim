//go:build !windows

package updater

func isWindows7OrEarlier() bool {
	return false
}
