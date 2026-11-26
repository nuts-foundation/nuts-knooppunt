package bsnutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	testCases := []struct {
		name, input, want string
	}{
		{"BSN", "unknown patient 123456782", "unknown patient [redacted BSN]"},
		{"BSN inside a token", "http://fhir.nl/fhir/NamingSystem/bsn|123456782", "http://fhir.nl/fhir/NamingSystem/bsn|[redacted BSN]"},
		{"BSN next to letters", "Patient/patient-900186021", "Patient/patient-[redacted BSN]"},
		{"every BSN", "123456782 and 900186021", "[redacted BSN] and [redacted BSN]"},
		{"9 digits that fail the elfproef", "order 123456789", "order 123456789"},
		{"8-digit URA", "ura|00000020", "ura|00000020"},
		{"10 digits", "1234567820", "1234567820"},
		{"no digits", "nothing to redact", "nothing to redact"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Redact(tc.input))
		})
	}
}
