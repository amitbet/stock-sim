//go:build darwin

package updater

import (
	"strings"
	"testing"
)

func TestBuildDarwinApplyScriptRelaunchesFreshInstance(t *testing.T) {
	script := buildDarwinApplyScript("/Applications", "/tmp/stock-sim.app")

	if !strings.Contains(script, `open -n "$DEST/$NAME" || true`) {
		t.Fatalf("expected relaunch to use open -n, script was:\n%s", script)
	}
	if !strings.Contains(script, `cp -R "$NEW" "$DEST/"`) {
		t.Fatalf("expected copied app bundle in script, script was:\n%s", script)
	}
}
