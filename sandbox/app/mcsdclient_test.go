package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcsdServer serves handler as HAPI and returns the base URL the sandbox is
// configured with, i.e. the FHIR base, not the query-directory tenant: picking
// the tenant out of it is the client's job and one of the things these tests pin.
func mcsdServer(t *testing.T, handler http.HandlerFunc) *url.URL {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL + "/fhir")
	require.NoError(t, err)
	return base
}

// organizationBundle is a searchset holding De Zonnebloem and, as an _include,
// one Endpoint. endpointJSON is spliced in so a test can vary the Endpoint alone.
func organizationBundle(endpointJSON string) string {
	return `{
	  "resourceType": "Bundle",
	  "type": "searchset",
	  "entry": [
	    {
	      "search": {"mode": "match"},
	      "resource": {
	        "resourceType": "Organization",
	        "id": "zonnebloem",
	        "name": "Zorgcentrum De Zonnebloem",
	        "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": "00000020"}],
	        "endpoint": [{"reference": "Endpoint/zb-fhir", "type": "Endpoint"}]
	      }
	    },
	    {"search": {"mode": "include"}, "resource": ` + endpointJSON + `}
	  ]
	}`
}

const activeEndpoint = `{
  "resourceType": "Endpoint",
  "id": "zb-fhir",
  "status": "active",
  "address": "http://pep-zonnebloem:8080/fhir",
  "connectionType": {"system": "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type", "code": "fhir"},
  "period": {"start": "2025-01-01T00:00:00Z"}
}`

func TestMCSDResolve_SearchesTheQueryDirectoryByURA(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	base := mcsdServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query()
		_, _ = w.Write([]byte(organizationBundle(activeEndpoint)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "/fhir/knpt-mcsd-query/Organization", gotPath)
	assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/ura|00000020", gotQuery.Get("identifier"))
	assert.Equal(t, "Organization:endpoint", gotQuery.Get("_include"))
	assert.Equal(t, "00000020", source.URA)
	assert.Equal(t, "Zorgcentrum De Zonnebloem", source.Name)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address)
}

// An Endpoint that is not active is not an address, and using it anyway would
// send the retrieval at a system its operator has taken out of service.
func TestMCSDResolve_RefusesAnEndpointThatIsNotActive(t *testing.T) {
	suspended := `{
	  "resourceType": "Endpoint", "id": "zb-fhir", "status": "suspended",
	  "address": "http://pep-zonnebloem:8080/fhir",
	  "connectionType": {"system": "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type", "code": "fhir"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(suspended)))
	})

	_, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.ErrorContains(t, err, "no usable endpoint")
}

// Endpoint.period is how a migration between systems is expressed: the old
// endpoint keeps a period.end at the cutover moment. Ignoring it would keep
// sending retrievals at the decommissioned system.
func TestMCSDResolve_RefusesAnEndpointWhosePeriodHasEnded(t *testing.T) {
	expired := `{
	  "resourceType": "Endpoint", "id": "zb-fhir", "status": "active",
	  "address": "http://pep-zonnebloem:8080/fhir",
	  "connectionType": {"system": "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type", "code": "fhir"},
	  "period": {"start": "2020-01-01T00:00:00Z", "end": "2021-01-01T00:00:00Z"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(expired)))
	})

	_, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.ErrorContains(t, err, "no usable endpoint")
}

// An Endpoint the Organization does not point at belongs to somebody else. The
// bundle carries it because _include is per-search, not per-resource.
func TestMCSDResolve_IgnoresAnEndpointTheOrganizationDoesNotReference(t *testing.T) {
	stranger := `{
	  "resourceType": "Endpoint", "id": "someone-else", "status": "active",
	  "address": "http://pep-elsewhere:8080/fhir",
	  "connectionType": {"system": "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type", "code": "fhir"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(stranger)))
	})

	_, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.ErrorContains(t, err, "no usable endpoint")
}

func TestMCSDResolve_UnknownOrganizationIsAnError(t *testing.T) {
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[]}`))
	})

	_, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.ErrorContains(t, err, "00000020")
}

func TestMCSDResolve_NonOKIsAnError(t *testing.T) {
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	_, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.ErrorContains(t, err, "502")
}
