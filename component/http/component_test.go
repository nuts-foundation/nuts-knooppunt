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
		instance1 := New(mux, mux)
		p1, _ := netutil.FreeTCPPort()
		p2, _ := netutil.FreeTCPPort()
		instance1.internalAddr = ":" + strconv.Itoa(p1)
		instance1.publicAddr = ":" + strconv.Itoa(p2)
		defer instance1.Stop(context.Background())
		err := instance1.Start()
		require.NoError(t, err)

		instance2 := New(mux, mux)
		instance2.internalAddr = instance1.internalAddr
		instance2.publicAddr = instance1.publicAddr
		defer instance2.Stop(context.Background())
		err = instance2.Start()
		require.ErrorContains(t, err, "address already in use")
	})
}
