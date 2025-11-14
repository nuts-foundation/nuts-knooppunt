package netutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFreeTCPPort(t *testing.T) {
	t.Run("2 ports, should be different", func(t *testing.T) {
		port1, err := FreeTCPPort()
		assert.NoError(t, err)
		port2, err := FreeTCPPort()
		assert.NoError(t, err)
		assert.NotEqual(t, port1, port2)
	})
}
