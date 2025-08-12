package main

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"sync"
	"syscall"
	"testing"
	"time"
)

func Test_Main(t *testing.T) {
	t.Log("This tests the application lifecycle, making sure it stops gracefully on SIGINT.")
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		main()
	}()
	if waitForUp(t) {
		return
	}
	t.Run("Check if Nuts node is running", func(t *testing.T) {
		t.Run("public endpoint status", func(t *testing.T) {
			resp, err := http.Get("http://localhost:8080/nuts/status")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK")
		})
		t.Run("internal endpoint status", func(t *testing.T) {
			resp, err := http.Get("http://localhost:8081/nuts/status")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK")
		})
	})
	// Shutdown: send interrupt signal
	err := syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	require.NoError(t, err)
	// Wait for the main goroutine to finish
	wg.Wait()
}

func waitForUp(t *testing.T) bool {
	// Wait for /status to be available on http://localhost:8080/status
	for i := 0; i < 10; i++ {
		resp, err := http.Get("http://localhost:8080/status")
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		t.Logf("Waiting for status endpoint to be available (%d/10)", i+1)
		if i < 9 {
			time.Sleep(1 * time.Second)
		} else {
			t.Error("Time-out waiting for status endpoint to be available")
			return true
		}
	}
	return false
}
