package mcsdadmin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Test_endpoints(t *testing.T) {
	t.Run("new", func(t *testing.T) {
		fhirClient := &test.StubFHIRClient{
			Resources: []any{fhir.Organization{
				Id: to.Ptr("org-1"),
			}},
		}
		component := Component{fhirClient: fhirClient}

		params := url.Values{
			"address":         {"https://example.com/mcsd"},
			"payload-type":    {"http://nuts-foundation.github.io/nl-generic-functions-ig/CapabilityStatement/nl-gf-admin-directory-update-client"},
			"connection-type": {"hl7-fhir-rest"},
			"status":          {"active"},
			"endpoint-for":    {"org-1"},
		}
		httpRequest := httptest.NewRequest(http.MethodPost, "/mcsdadmin/endpoint/new", strings.NewReader(params.Encode()))
		httpRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		httpResponse := httptest.NewRecorder()

		component.newEndpointPost(httpResponse, httpRequest)

		assert.Equal(t, http.StatusCreated, httpResponse.Result().StatusCode)
		assert.Contains(t, httpResponse.Body.String(), "html")
	})
	t.Run("delete", func(t *testing.T) {
		fhirClient := &test.StubFHIRClient{
			Resources: []any{fhir.Endpoint{
				Id: to.Ptr("ep-1"),
			}},
		}
		component := Component{fhirClient: fhirClient}
		httpRequest := httptest.NewRequest(http.MethodDelete, "/mcsdadmin/endpoint/ep-1", nil)
		httpRequest.SetPathValue("id", "ep-1")
		httpResponse := httptest.NewRecorder()

		component.deleteHandler("Endpoint")(httpResponse, httpRequest)

		assert.Equal(t, http.StatusOK, httpResponse.Result().StatusCode)
		assert.Empty(t, httpResponse.Body.String())
	})
}
