package bsnutil

import "testing"

func TestValidElfproef(t *testing.T) {
	tests := []struct {
		name string
		bsn  string
		want bool
	}{
		// Valid BSNs (pass the 11-proof).
		{"sunflower fixture", "999992193", true},
		{"anna placeholder", "999911120", true},
		{"pip fixture", "176286603", true},
		{"pep fixture", "900186021", true},
		{"synthetic pool bsn", "999900006", true},
		{"repeated digits that happen to pass", "111111110", true},

		// Invalid: wrong length.
		{"too short", "12345", false},
		{"too long", "1234567890", false},
		{"empty", "", false},

		// Invalid: non-digit.
		{"trailing letter", "99999219a", false},
		{"embedded space", "99999 193", false},

		// Invalid: fails the checksum.
		{"off by one", "999911121", false},

		// Invalid: all zeros weights to 0, which is a multiple of 11 but not a BSN.
		{"all zeros", "000000000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidElfproef(tt.bsn); got != tt.want {
				t.Errorf("ValidElfproef(%q) = %v, want %v", tt.bsn, got, tt.want)
			}
		})
	}
}
