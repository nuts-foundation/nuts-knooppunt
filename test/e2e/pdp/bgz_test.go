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

func Test_BGZAuthorization(t *testing.T) {
	harnessDetails := harness.Start(t)

	pdpBaseUrl := harnessDetails.KnooppuntInternalBaseURL
	pdpBaseUrl.Path = pdpBaseUrl.Path + "/pdp"

	t.Run("authorize complete bgz request using the PDP", func(t *testing.T) {
		pdpJSON := `{
		  "input": {
			"subject": {
			  "properties": {
				"subject_id": "000095254",
				"subject_role": "01.015",
				"subject_organization_id": "00000666",
				"subject_facility_type": "Z3",
				"client_qualifications": ["bgz"]
			  }
			},
			"request": {
			  "method": "GET",
			  "protocol": "HTTP/1.0",
			  "path": "/Patient",
			  "query_params": {
 			    "_include": ["Patient:general-practitioner"],
				"_id": ["3E439979-017F-40AA-594D-EBCF880FFD97"]
              }
			},
			"context": {
			  "data_holder_organization_id": "00000659",
			  "data_holder_facility_type": "Z3",
              "connection_type_code": "hl7-fhir-rest",
              "patient_bsn": ""
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

		var pdpResponse pdp.PDPResponse
		responseData, _ := io.ReadAll(resp.Body)
		err = json.NewDecoder(bytes.NewReader(responseData)).Decode(&pdpResponse)
		require.NoError(t, err)

		err = resp.Body.Close()
		require.NoError(t, err)

		assert.True(t, pdpResponse.Allow)
		assert.Empty(t, pdpResponse.Reasons)
	})
}
