package harness

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/test"
)

func startKnooppunt(t *testing.T, config cmd.Config) *url.URL {
	t.Helper()

	var errChan = make(chan error, 1)
	go func() {
		if err := cmd.Start(t.Context(), config); err != nil {
			errChan <- err
		}
	}()

	baseURL, _ := url.Parse(config.HTTP.InternalInterface.BaseURL)
	doneChan, timeoutChan := test.WaitForHTTPStatus(baseURL.JoinPath("status").String(), http.StatusOK)
	select {
	case err := <-errChan:
		t.Fatalf("failed to start knooppunt: %v", err)
	case <-doneChan:
		t.Log("Knooppunt started successfully")
	case err := <-timeoutChan:
		t.Fatalf("timeout waiting for knooppunt to start: %v", err)
	}
	return baseURL
}
