package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func getPage(t *testing.T, path string) (int, string) {
	t.Helper()
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + path)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

func TestHealthz(t *testing.T) {
	status, body := getPage(t, "/healthz")
	require.Equal(t, http.StatusOK, status)
	require.JSONEq(t, `{"ok": true}`, body)
}
