//go:build darwin

package updater

import "testing"

func TestDarwinUpdateBlockedReason(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"/private/var/folders/xx/AppTranslocation/abc/stock-sim.app", true},
		{"/Volumes/MyVol/stock-sim.app", true},
		{"/Users/me/Applications/stock-sim.app", false},
		{"/Users/me/workspace/stock-sim/build/bin/stock-sim.app", false},
	}
	for _, tc := range tests {
		err := darwinUpdateBlockedReason(tc.path)
		if tc.wantErr && err == nil {
			t.Errorf("%q: want error, got nil", tc.path)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%q: want nil, got %v", tc.path, err)
		}
	}
}
