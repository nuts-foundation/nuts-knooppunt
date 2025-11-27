package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("strict mode is on by default", func(t *testing.T) {
		assert.True(t, DefaultConfig().StrictMode)
	})
}
