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
		err   string
	}{
		{"url-safe alphabet", "-_8=", "fbff", ""},
		{"standard alphabet", "+/8=", "fbff", ""},
		{"characters outside the alphabet are discarded", "+!/ 8\n=", "fbff", ""},
		{"padding in the first two positions of a quad is ignored", "=+=/8=", "fbff", ""},
		{"decoding stops after complete padding", "+/8=AAAA", "fbff", ""},
		{"decoding stops after complete double padding", "AA==BBBB", "00", ""},
		{"empty", "", "", ""},
		{"missing padding", "+/8", "", "Incorrect padding"},
		{"incomplete padding", "+/=", "", "Incorrect padding"},
		{"padding interrupted by data", "AA=A", "", "Incorrect padding"},
		{"one data character too many", "AAAAA", "", "Invalid base64-encoded string: number of data characters (5) cannot be 1 more than a multiple of 4"},
		{"non-ASCII", "+/8=é", "", "string argument should contain only ASCII characters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := pythonURLSafeB64Decode(tc.input)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.hex, hex.EncodeToString(decoded))
		})
	}
}
