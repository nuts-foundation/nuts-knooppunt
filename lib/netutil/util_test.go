package netutil

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFreeTCPPort(t *testing.T) {
	t.Run("2 ports, should be different", func(t *testing.T) {
		port1 := FreeTCPPort()
		port2 := FreeTCPPort()
		assert.NotEqual(t, port1, port2)
	})
}
