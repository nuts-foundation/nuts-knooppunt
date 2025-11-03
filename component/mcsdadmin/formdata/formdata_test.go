package formdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostForm(t *testing.T) {
	postform := map[string][]string{
		"telecom[0][System]":   []string{"phone"},
		"telecom[0][value]":    []string{"+31612345678"},
		"telecom[1][System]":   []string{"email"},
		"telecom[1][value]":    []string{"foo@bar.nl"},
		"reference[0][system]": []string{"email"},
		"reference[0][value]":  []string{"baz@qux.nl"},
	}

	result := ParseMaps(postform, "telecom")
	require.Equal(t, 2, len(result), "Expected two telecom entries")
}
