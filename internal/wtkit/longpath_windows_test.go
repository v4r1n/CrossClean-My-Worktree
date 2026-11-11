package wtkit

import "testing"

func TestLongPath(t *testing.T) {
	cases := map[string]string{
		`C:\a\b`:         `\\?\C:\a\b`,
		`C:\a\..\b`:      `\\?\C:\b`,
		`\\server\share`: `\\?\UNC\server\share`,
		`\\?\C:\already`: `\\?\C:\already`,
		`relative\path`:  `relative\path`,
	}
	for in, want := range cases {
		if got := longPath(in); got != want {
			t.Errorf("longPath(%q) = %q, want %q", in, got, want)
		}
	}
	if clusterSize(`Z:\definitely-missing`) <= 0 {
		t.Error("clusterSize should fall back to a positive default")
	}
}
