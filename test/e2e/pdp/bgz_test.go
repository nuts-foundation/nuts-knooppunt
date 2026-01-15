package pdp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/require"
)

func Test_BGZAuthorization(t *testing.T) {
	harnessDetails := harness.Start(t)

	pdpBaseUrl := harnessDetails.KnooppuntInternalBaseURL
	pdpBaseUrl.Path = pdpBaseUrl.Path + "/pdp"

	t.Run("authorize complete bgz request using the PDP", func(t *testing.T) {

		pdpJSON := `{
		"input": {
			"subject": {},
			"request": {},
			"context": {}
		}
		}`

		// Make request to PDP
		req, err := http.NewRequest(
			"POST",
			pdpBaseUrl.JoinPath("v1", "data", "knooppunt", "authz").String(),
			strings.NewReader(pdpJSON),
		)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		var pdpResponse pdp.PDPResponse
		err = json.NewDecoder(resp.Body).Decode(&pdpResponse)
		require.NoError(t, err)
		defer resp.Body.Close()
	})
}
