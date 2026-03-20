package pdp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_EOverdrachtAuthorize(t *testing.T) {
	harnessDetails := harness.Start(t)

	pdpBaseUrl := harnessDetails.KnooppuntInternalBaseURL
	pdpBaseUrl.Path = pdpBaseUrl.Path + "/pdp"

	t.Run("authorize valid e-overdracht request using the PDP", func(t *testing.T) {
		pdpJSON := `{
		  "input": {
			"subject": {
			  "user_id": "000095254",
			  "user_role": "01.015",
			  "organization_ura": "00000040",
			  "organization_facility_type": "Z3",
			  "scope": "eoverdracht-receiver"
			},
			"request": {
			  "method": "GET",
			  "protocol": "HTTP/1.0",
			  "path": "/Observation/7DC623BA-0EF1-42AD-0AAD-F4D034F67C9F"
			},
			"context": {
			  "data_holder_organization_id": "00000030",
			  "data_holder_facility_type": "Z3",
			  "connection_type_code": "hl7-fhir-rest"
			}
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

		var pdpResponse pdp.APIResponse
		responseData, _ := io.ReadAll(resp.Body)
		err = json.NewDecoder(bytes.NewReader(responseData)).Decode(&pdpResponse)
		require.NoError(t, err)

		err = resp.Body.Close()
		require.NoError(t, err)

		assert.True(t, pdpResponse.Allow)
		assert.Empty(t, pdpResponse.Error)
	})

	t.Run("reject request using unknown data holder", func(t *testing.T) {
		pdpJSON := `{
		  "input": {
			"subject": {
			  "user_id": "000095254",
			  "user_role": "01.015",
			  "organization_ura": "00000040",
			  "organization_facility_type": "Z3",
			  "scope": "eoverdracht-receiver"
			},
			"request": {
			  "method": "GET",
			  "protocol": "HTTP/1.0",
			  "path": "/Observation/7DC623BA-0EF1-42AD-0AAD-F4D034F67C9F"
			},
			"context": {
			  "data_holder_organization_id": "00000031",
			  "data_holder_facility_type": "Z3",
			  "connection_type_code": "hl7-fhir-rest"
			}
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

		var pdpResponse pdp.APIResponse
		responseData, _ := io.ReadAll(resp.Body)
		err = json.NewDecoder(bytes.NewReader(responseData)).Decode(&pdpResponse)
		require.NoError(t, err)

		err = resp.Body.Close()
		require.NoError(t, err)

		assert.False(t, pdpResponse.Allow)
		assert.Empty(t, pdpResponse.Error)
	})

	t.Run("reject request using unknown subject organization", func(t *testing.T) {
		pdpJSON := `{
		  "input": {
			"subject": {
			  "user_id": "000095254",
			  "user_role": "01.015",
			  "organization_ura": "00000041",
			  "organization_facility_type": "Z3",
			  "scope": "eoverdracht-receiver"
			},
			"request": {
			  "method": "GET",
			  "protocol": "HTTP/1.0",
			  "path": "/Observation/7DC623BA-0EF1-42AD-0AAD-F4D034F67C9F"
			},
			"context": {
			  "data_holder_organization_id": "00000030",
			  "data_holder_facility_type": "Z3",
			  "connection_type_code": "hl7-fhir-rest"
			}
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

		var pdpResponse pdp.APIResponse
		responseData, _ := io.ReadAll(resp.Body)
		err = json.NewDecoder(bytes.NewReader(responseData)).Decode(&pdpResponse)
		require.NoError(t, err)

		err = resp.Body.Close()
		require.NoError(t, err)

		assert.False(t, pdpResponse.Allow)
		assert.Empty(t, pdpResponse.Error)
	})
}
