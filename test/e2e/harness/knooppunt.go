package harness

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/test"
)

func startKnooppunt(t *testing.T, ctx context.Context, config cmd.Config) (*url.URL, chan struct{}) {
	t.Helper()
	wg := sync.WaitGroup{}
	wg.Add(1)
	var errChan = make(chan error, 1)
	var shutdownChan = make(chan struct{}, 1)
	go func() {
		defer wg.Done()
		if err := cmd.Start(ctx, config); err != nil {
			errChan <- err
		}
		shutdownChan <- struct{}{}
	}()

	baseURL, _ := url.Parse("http://localhost:8081")
	doneChan, timeoutChan := test.WaitForHTTPStatus(baseURL.JoinPath("status").String(), http.StatusOK)
	select {
	case err := <-errChan:
		t.Fatalf("failed to start knooppunt: %v", err)
	case <-doneChan:
		t.Log("Knooppunt started successfully")
	case err := <-timeoutChan:
		t.Fatalf("timeout waiting for knooppunt to start: %v", err)
	}
	return baseURL, shutdownChan
}
