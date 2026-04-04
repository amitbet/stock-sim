//go:build !darwin

package updater

func preflightApplyInstallLocation() error {
	return nil
}
