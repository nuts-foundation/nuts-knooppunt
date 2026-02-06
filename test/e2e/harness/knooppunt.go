package harness

import (
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/test"
)

// startKnooppunt starts Knooppunt with the given config and waits for it to be ready.
// If Nuts is enabled in the config, it also waits for the embedded Nuts node to be ready.
func startKnooppunt(t *testing.T, config cmd.Config) *url.URL {
	t.Helper()

	// Clean the hardcoded Nuts data directory if Nuts is enabled
	if config.Nuts.Enabled {
		if err := os.RemoveAll("data/nuts"); err != nil && !os.IsNotExist(err) {
			t.Logf("Warning: failed to clean up data/nuts: %v", err)
		}
	}

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

	// If Nuts is enabled, also wait for the embedded Nuts node to be ready
	if config.Nuts.Enabled {
		nutsDoneChan, nutsTimeoutChan := test.WaitForHTTPStatus(baseURL.JoinPath("/nuts/status").String(), http.StatusOK)
		select {
		case <-nutsDoneChan:
			t.Log("Embedded Nuts node ready")
		case err := <-nutsTimeoutChan:
			t.Fatalf("timeout waiting for embedded Nuts node: %v", err)
		}
	}

	return baseURL
}
