package main

import (
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/from"
	"github.com/stretchr/testify/require"
)

func Test_Main(t *testing.T) {
	t.Log("This tests the application lifecycle, making sure it stops gracefully on SIGINT.")

	os.Setenv("NUTS_POLICY_DIRECTORY", "./config/policy")

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		main()
	}()
	if waitForUp(t) {
		return
	}
	publicURL := "http://localhost:8080"
	privateURL := "http://localhost:8081"

	t.Run("check if Nuts node is running", func(t *testing.T) {
		subjectID, dids, err := createNutsSubject(privateURL)
		require.NoError(t, err)
		t.Run("public endpoint status", func(t *testing.T) {
			resp, err := http.Get(publicURL + "/nuts/status")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK")
		})
		t.Run("internal endpoint status", func(t *testing.T) {
			resp, err := http.Get(privateURL + "/nuts/status")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK")
		})
		t.Run("query well-known openid-configuration of Nuts node", func(t *testing.T) {
			resp, err := http.Get(publicURL + "/.well-known/oauth-authorization-server/nuts/oauth2/" + subjectID)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK")
		})
		t.Run("resolve created DID", func(t *testing.T) {
			for _, currentDID := range dids {
				resp, err := http.Get(privateURL + "/nuts/internal/vdr/v2/did/" + currentDID)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK for DID "+currentDID)
			}
		})
	})
	// Shutdown: send interrupt signal
	err := syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	require.NoError(t, err)
	// Wait for the main goroutine to finish
	wg.Wait()
}

func waitForUp(t *testing.T) bool {
	// Wait for both knooppunt and the embedded nuts node to be up
	for i := 0; i < 10; i++ {
		retry := false

		knpt_resp, knpt_err := http.Get("http://localhost:8081/status")
		nuts_resp, nuts_err := http.Get("http://localhost:8081/nuts/status")

		if knpt_err != nil || knpt_resp.StatusCode != http.StatusOK {
			t.Logf("Waiting for knooppunt endpoint to be available (%d/10)", i+1)
			retry = true
		} else if nuts_err != nil || nuts_resp.StatusCode != http.StatusOK {
			t.Logf("Waiting for nuts endpoint to be available (%d/10)", i+1)
			retry = true
		}

		if !retry {
			break
		}

		if i < 9 {
			time.Sleep(1 * time.Second)
		} else {
			t.Fatal("Time-out waiting for status endpoint to be available")
			return true
		}
	}
	return false
}

func createNutsSubject(privateURL string) (string, []string, error) {
	httpResponse, err := http.Post(privateURL+"/nuts/internal/vdr/v2/subject", "application/json", nil)
	if err != nil {
		return "", nil, err
	}
	response, err := from.JSONResponse[map[string]any](httpResponse)
	if err != nil {
		return "", nil, err
	}
	subjectID := response["subject"].(string)
	var dids []string
	for _, didDocument := range response["documents"].([]any) {
		dids = append(dids, didDocument.(map[string]any)["id"].(string))
	}
	return subjectID, dids, nil
}
