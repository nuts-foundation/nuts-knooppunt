package http

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/netutil"
	"github.com/stretchr/testify/require"
)

func TestComponent_Start(t *testing.T) {
	t.Run("bind address already in use", func(t *testing.T) {
		mux := http.NewServeMux()
		p1, _ := netutil.FreeTCPPort()
		p2, _ := netutil.FreeTCPPort()
		cfg := Config{
			InternalInterface: InterfaceConfig{
				Address: ":" + strconv.Itoa(p1),
			},
			PublicInterface: InterfaceConfig{
				Address: ":" + strconv.Itoa(p2),
			},
		}
		instance1 := New(cfg, mux, mux)
		defer instance1.Stop(context.Background())
		err := instance1.Start()
		require.NoError(t, err)

		instance2 := New(cfg, mux, mux)
		defer instance2.Stop(context.Background())
		err = instance2.Start()
		require.ErrorContains(t, err, "address already in use")
	})
}
