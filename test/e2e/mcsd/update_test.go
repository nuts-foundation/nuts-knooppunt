package mcsd

import (
	"io"
	"net/http"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/require"
)

func Test_mCSDUpdateClient(t *testing.T) {
	harnessDetail := harness.Start(t)
	t.Run("Force update mCSD Client", func(t *testing.T) {
		httpResponse, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse.StatusCode)
		responseData, err := io.ReadAll(httpResponse.Body)
		require.NoError(t, err)
		println(string(responseData))
	})
}
