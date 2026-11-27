package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/netutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfigOnFreePorts(t *testing.T) Config {
	t.Helper()
	p1, err := netutil.FreeTCPPort()
	require.NoError(t, err)
	p2, err := netutil.FreeTCPPort()
	require.NoError(t, err)
	cfg := DefaultConfig()
	cfg.InternalInterface.Address = "localhost:" + strconv.Itoa(p1)
	cfg.PublicInterface.Address = "localhost:" + strconv.Itoa(p2)
	return cfg
}

func TestComponent_Start(t *testing.T) {
	t.Run("bind address already in use", func(t *testing.T) {
		mux := http.NewServeMux()
		cfg := testConfigOnFreePorts(t)
		instance1 := New(cfg, mux, mux)
		defer instance1.Stop(context.Background())
		err := instance1.Start()
		require.NoError(t, err)

		instance2 := New(cfg, mux, mux)
		defer instance2.Stop(context.Background())
		err = instance2.Start()
		require.ErrorContains(t, err, "address already in use")
	})
	t.Run("max body size must be positive", func(t *testing.T) {
		mux := http.NewServeMux()
		for _, size := range []int64{0, -1} {
			cfg := testConfigOnFreePorts(t)
			cfg.PublicInterface.MaxBodySize = size
			err := New(cfg, mux, mux).Start()
			assert.ErrorContains(t, err, "http.public.maxbodysize must be greater than 0")

			cfg = testConfigOnFreePorts(t)
			cfg.InternalInterface.MaxBodySize = size
			err = New(cfg, mux, mux).Start()
			assert.ErrorContains(t, err, "http.internal.maxbodysize must be greater than 0")
		}
	})
	t.Run("limits", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			_, _ = w.Write([]byte(strconv.Itoa(len(data))))
		})
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {})
		cfg := testConfigOnFreePorts(t)
		instance := New(cfg, mux, mux)
		require.NoError(t, instance.Start())
		defer instance.Stop(context.Background())

		post := func(t *testing.T, baseURL string, size int64) *http.Response {
			t.Helper()
			response, err := http.Post(baseURL+"/echo", "application/octet-stream", strings.NewReader(strings.Repeat("a", int(size))))
			require.NoError(t, err)
			t.Cleanup(func() { _ = response.Body.Close() })
			return response
		}

		t.Run("body ceiling per interface", func(t *testing.T) {
			for name, iface := range map[string]InterfaceConfig{"public": cfg.PublicInterface, "internal": cfg.InternalInterface} {
				t.Run(name, func(t *testing.T) {
					baseURL := "http://" + iface.Address
					assert.Equal(t, http.StatusOK, post(t, baseURL, iface.MaxBodySize).StatusCode)
					assert.Equal(t, http.StatusRequestEntityTooLarge, post(t, baseURL, iface.MaxBodySize+1).StatusCode)
				})
			}
		})
		t.Run("headers over the limit get 431", func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "http://"+cfg.PublicInterface.Address+"/", nil)
			require.NoError(t, err)
			// net/http allows 4 KiB of slop on top of MaxHeaderBytes before it answers 431.
			request.Header.Set("X-Large", strings.Repeat("a", maxHeaderBytes+8<<10))
			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			assert.Equal(t, http.StatusRequestHeaderFieldsTooLarge, response.StatusCode)
		})
		t.Run("headers under the limit pass", func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "http://"+cfg.PublicInterface.Address+"/", nil)
			require.NoError(t, err)
			request.Header.Set("X-Large", strings.Repeat("a", 32<<10))
			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			assert.Equal(t, http.StatusOK, response.StatusCode)
		})
	})
}

func Test_limitBody(t *testing.T) {
	const limit = 16
	var readErr error
	var readLen int
	handler := limitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		data, readErr = io.ReadAll(r.Body)
		readLen = len(data)
	}), limit)

	t.Run("body at the limit passes", func(t *testing.T) {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", limit))))
		assert.NoError(t, readErr)
		assert.Equal(t, limit, readLen)
	})
	t.Run("body one byte over the limit gives a MaxBytesError", func(t *testing.T) {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", limit+1))))
		var tooLarge *http.MaxBytesError
		require.ErrorAs(t, readErr, &tooLarge)
		assert.Equal(t, int64(limit), tooLarge.Limit)
	})
	t.Run("no body is left alone", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(httptest.NewRecorder(), request)
		assert.Equal(t, http.NoBody, request.Body)
	})
}
