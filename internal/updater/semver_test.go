package updater

import "testing"

func TestParseSemver(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"1.0.0", true},
		{"v1.2.3", true},
		{"1.2.3-rc.1", true},
		{"1.2.3+build.5", true},
		{"1.2", false},
		{"1.02.3", false},
		{"", false},
	}

	for _, tc := range tests {
		_, ok := parseSemver(tc.v)
		if ok != tc.want {
			t.Fatalf("parseSemver(%q): got %v want %v", tc.v, ok, tc.want)
		}
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		left string
		right string
		want int
	}{
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-rc.2", "1.0.0-rc.1", 1},
		{"2.0.0", "10.0.0", -1},
	}

	for _, tc := range tests {
		got := compareSemver(tc.left, tc.right)
		switch {
		case got < 0:
			got = -1
		case got > 0:
			got = 1
		}
		if got != tc.want {
			t.Fatalf("compareSemver(%q, %q): got %d want %d", tc.left, tc.right, got, tc.want)
		}
	}
}
