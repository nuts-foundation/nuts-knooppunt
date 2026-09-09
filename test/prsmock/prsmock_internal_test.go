package prsmock

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_pythonURLSafeB64Decode(t *testing.T) {
	// Expected values are base64.urlsafe_b64decode(input) on CPython 3.11.6,
	// the Python line the service runs (pyproject.toml at v0.0.18).
	for _, tc := range []struct {
		name  string
		input string
		hex   string
		err   bool
	}{
		{"url-safe alphabet", "-_8=", "fbff", false},
		{"standard alphabet", "+/8=", "fbff", false},
		{"characters outside the alphabet are discarded", "+!/ 8\n=", "fbff", false},
		{"padding in the first two positions of a quad is ignored", "=+=/8=", "fbff", false},
		{"decoding stops after complete padding", "+/8=AAAA", "fbff", false},
		{"decoding stops after complete double padding", "AA==BBBB", "00", false},
		{"empty", "", "", false},
		{"missing padding", "+/8", "", true},
		{"incomplete padding", "+/=", "", true},
		{"padding interrupted by data", "AA=A", "", true},
		{"one data character too many", "AAAAA", "", true},
		{"non-ASCII", "+/8=é", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := pythonURLSafeB64Decode(tc.input)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.hex, hex.EncodeToString(decoded))
		})
	}
}
