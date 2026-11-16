package logging

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactURL(t *testing.T) {
	parse := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	assert.Equal(t, "https://nvi.example/fhir/List?[redacted]", RedactURL(parse("https://user:pass@nvi.example/fhir/List?subject:identifier=x|123#frag")))
	assert.Equal(t, "https://prs.example/oprf/eval", RedactURL(parse("https://prs.example/oprf/eval")))
	assert.Equal(t, "", RedactURL(nil))
}
