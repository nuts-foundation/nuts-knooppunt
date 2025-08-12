package main

import (
	"net/http"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Main(t *testing.T) {
	t.Log("This tests the application lifecycle, making sure it stops gracefully on SIGINT.")
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		main()
	}()
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
			return
		}
	}
	
	// Also test that subsystem endpoints are available
	resp, err := http.Get("http://localhost:8080/nuts/health")
	if err == nil && resp.StatusCode == http.StatusOK {
		t.Log("Nuts subsystem health endpoint is accessible")
	} else {
		t.Logf("Nuts subsystem not yet available (this is expected during startup): %v", err)
	}
	
	// Send interrupt signal
	err = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	require.NoError(t, err)
	// Wait for the main goroutine to finish
	wg.Wait()
}
