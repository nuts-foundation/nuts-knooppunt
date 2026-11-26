package logging

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactURL(t *testing.T) {
	testCases := []struct {
		name, input, want string
	}{
		{
			name:  "query values are redacted, names kept; user info and fragment dropped",
			input: "https://user:pass@nvi.example/fhir/List?subject:identifier=x|123&_count=20#frag",
			want:  "https://nvi.example/fhir/List?subject:identifier=[redacted]&_count=[redacted]",
		},
		{
			name:  "encoded names stay encoded",
			input: "http://nvi.example/List?subject%3Aidentifier=token-abc",
			want:  "http://nvi.example/List?subject%3Aidentifier=[redacted]",
		},
		{
			name:  "a parameter without a name or '=' is redacted as a whole",
			input: "https://pip.example/fhir/Patient?123456782&=x",
			want:  "https://pip.example/fhir/Patient?[redacted]&=[redacted]",
		},
		{
			name:  "a BSN in the path is redacted, other ids are kept",
			input: "https://pip.example/fhir/Patient/123456782/Consent/abc-1",
			want:  "https://pip.example/fhir/Patient/[redacted BSN]/Consent/abc-1",
		},
		{
			name:  "no query",
			input: "https://prs.example/oprf/eval",
			want:  "https://prs.example/oprf/eval",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.want, RedactURL(u))
		})
	}
	t.Run("nil", func(t *testing.T) {
		assert.Equal(t, "", RedactURL(nil))
	})
}
