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
	return organizationBundleWith([]string{"Endpoint/zb-fhir"}, endpointJSON)
}

// organizationBundleWith is the same searchset with the Organization pointing at
// several endpoints, which is the shape once a holder publishes both where its
// data is and where its authorization server is.
func organizationBundleWith(references []string, endpointsJSON ...string) string {
	refs := ""
	for i, reference := range references {
		if i > 0 {
			refs += ","
		}
		refs += `{"reference": "` + reference + `", "type": "Endpoint"}`
	}
	included := ""
	for _, endpoint := range endpointsJSON {
		included += `,{"search": {"mode": "include"}, "resource": ` + endpoint + `}`
	}
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
	        "endpoint": [` + refs + `]
	      }
	    }` + included + `
	  ]
	}`
}

const authServerEndpoint = `{
  "resourceType": "Endpoint",
  "id": "zb-oauth",
  "status": "active",
  "address": "http://localhost:8080/nuts/oauth2/00000020",
  "connectionType": {"system": "http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-authorization-server-cs", "code": "oauth-nuts"},
  "period": {"start": "2025-01-01T00:00:00Z"}
}`

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
func TestMCSDResolve_DoesNotAddressAnEndpointThatIsNotActive(t *testing.T) {
	suspended := `{
	  "resourceType": "Endpoint", "id": "zb-fhir", "status": "suspended",
	  "address": "http://pep-zonnebloem:8080/fhir",
	  "connectionType": {"system": "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type", "code": "fhir"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(suspended)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err, "the organization was found; what it publishes is the answer")
	require.Empty(t, source.Address)
}

// Endpoint.period is how a migration between systems is expressed: the old
// endpoint keeps a period.end at the cutover moment. Ignoring it would keep
// sending retrievals at the decommissioned system.
func TestMCSDResolve_DoesNotAddressAnEndpointWhosePeriodHasEnded(t *testing.T) {
	expired := `{
	  "resourceType": "Endpoint", "id": "zb-fhir", "status": "active",
	  "address": "http://pep-zonnebloem:8080/fhir",
	  "connectionType": {"system": "http://fhir.nl/fhir/NamingSystem/endpoint-connection-type", "code": "fhir"},
	  "period": {"start": "2020-01-01T00:00:00Z", "end": "2021-01-01T00:00:00Z"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(expired)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err, "the organization was found; what it publishes is the answer")
	require.Empty(t, source.Address)
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

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err, "the organization was found; what it publishes is the answer")
	require.Empty(t, source.Address)
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

// FHIR dateTime allows less precision than a full timestamp, and a Period bound
// written as a date is the common case in a directory. Reading only RFC 3339 and
// treating everything else as absent meant an endpoint that was taken out of
// service years ago still looked valid, which is the opposite of what the
// period is for.
func TestMCSDResolve_HonoursDateOnlyPeriodBounds(t *testing.T) {
	for _, tc := range []struct {
		name, period string
		usable       bool
	}{
		{"ended years ago, date only", `{"start":"2020-01-01","end":"2021-01-01"}`, false},
		{"ended years ago, year only", `{"start":"2020","end":"2021"}`, false},
		{"ended years ago, month only", `{"start":"2020-01","end":"2021-01"}`, false},
		{"starts far in the future", `{"start":"2099-01-01"}`, false},
		{"started long ago, open ended", `{"start":"2020-01-01"}`, true},
		{"ends far in the future", `{"start":"2020-01-01","end":"2099-01-01"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := `{"resourceType":"Endpoint","id":"zb-fhir","status":"active",
			  "address":"http://pep-zonnebloem:8080/fhir",
			  "connectionType":{"system":"http://fhir.nl/fhir/NamingSystem/endpoint-connection-type","code":"fhir"},
			  "period":` + tc.period + `}`
			base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(organizationBundle(endpoint)))
			})

			source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

			require.NoError(t, err)
			if tc.usable {
				require.NotEmpty(t, source.Address)
				return
			}
			require.Empty(t, source.Address)
		})
	}
}

// A literal reference may be absolute rather than relative
// (https://hl7.org/fhir/R4/references.html#literal), in which case it matches
// the included entry's fullUrl and not a Type/id. Keying only on Type/id
// reported a directory that answered perfectly well as holding no endpoint, and
// the retrieval this addressing step feeds never ran. The medication resolver in
// retrieval.go already indexes both.
func TestMCSDResolve_ResolvesAnEndpointReferencedByAbsoluteURL(t *testing.T) {
	const absolute = "https://directory.example/fhir/Endpoint/zb-fhir"
	bundle := `{
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
	        "endpoint": [{"reference": "` + absolute + `", "type": "Endpoint"}]
	      }
	    },
	    {"search": {"mode": "include"}, "fullUrl": "` + absolute + `", "resource": ` + activeEndpoint + `}
	  ]
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bundle))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address)
}

// The authorization server is a published endpoint, not something to build out
// of a URA. The GF connection types value set has oauth-nuts for exactly this
// (CodeSystem/nl-gf-authorization-server-cs), so the directory answers both
// halves of "where do I reach this holder": the data, and the server that
// authorizes access to it.
func TestMCSDResolve_ReadsTheAuthorizationServerFromTheDirectory(t *testing.T) {
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundleWith(
			[]string{"Endpoint/zb-fhir", "Endpoint/zb-oauth"}, activeEndpoint, authServerEndpoint)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address,
		"the data address may not be taken from the authorization endpoint")
	assert.Equal(t, "http://localhost:8080/nuts/oauth2/00000020", source.AuthorizationServer)
}

// The order the directory happens to return them in is not a selection rule.
func TestMCSDResolve_TellsTheTwoEndpointsApartInEitherOrder(t *testing.T) {
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundleWith(
			[]string{"Endpoint/zb-oauth", "Endpoint/zb-fhir"}, authServerEndpoint, activeEndpoint)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address)
	assert.Equal(t, "http://localhost:8080/nuts/oauth2/00000020", source.AuthorizationServer)
}

// A holder that publishes where its data is but not what authorizes access to it
// is still addressable, and the screen says what is missing rather than the
// retrieval inventing a server to ask.
func TestMCSDResolve_LeavesTheAuthorizationServerEmptyWhenNonePublished(t *testing.T) {
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(activeEndpoint)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address)
	assert.Empty(t, source.AuthorizationServer)
}

// An authorization endpoint that is out of service is not an address either.
func TestMCSDResolve_IgnoresAnAuthorizationEndpointThatIsNotUsable(t *testing.T) {
	suspended := `{
	  "resourceType": "Endpoint", "id": "zb-oauth", "status": "suspended",
	  "address": "http://localhost:8080/nuts/oauth2/00000020",
	  "connectionType": {"system": "http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-authorization-server-cs", "code": "oauth-nuts"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundleWith(
			[]string{"Endpoint/zb-fhir", "Endpoint/zb-oauth"}, activeEndpoint, suspended)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address)
	assert.Empty(t, source.AuthorizationServer)
}

// A directory entry is not limited to the two kinds this chain knows. Treating
// everything that is not an authorization server as the place to read data from
// hands the bearer token and the patient search, BSN and all, to whatever
// service the organization happened to list first.
func TestMCSDResolve_IgnoresAnEndpointKindItCannotUse(t *testing.T) {
	imaging := `{
	  "resourceType": "Endpoint", "id": "zb-imaging", "status": "active",
	  "address": "http://pacs-zonnebloem:8080/wado",
	  "connectionType": {"system": "http://terminology.hl7.org/CodeSystem/endpoint-connection-type", "code": "dicom-wado-rs"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundleWith(
			[]string{"Endpoint/zb-imaging", "Endpoint/zb-fhir", "Endpoint/zb-oauth"},
			imaging, activeEndpoint, authServerEndpoint)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address,
		"an imaging service is not where this reads a patient summary")
	assert.Equal(t, "http://localhost:8080/nuts/oauth2/00000020", source.AuthorizationServer)
}

// The spec's connection types draw FHIR REST from the HL7 code system; the
// seeded endpoints in this repo use a system that is in neither value set. Both
// are accepted so the demo keeps working while the seed is what it is.
func TestMCSDResolve_AcceptsTheSpecFHIRConnectionType(t *testing.T) {
	spec := `{
	  "resourceType": "Endpoint", "id": "zb-fhir", "status": "active",
	  "address": "http://pep-zonnebloem:8080/fhir",
	  "connectionType": {"system": "http://terminology.hl7.org/CodeSystem/endpoint-connection-type", "code": "hl7-fhir-rest"}
	}`
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundle(spec)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err)
	assert.Equal(t, "http://pep-zonnebloem:8080/fhir", source.Address)
}

// An organization publishing only an authorization server is not addressable for
// data, and saying so is different from silently using that server as a FHIR base.
func TestMCSDResolve_ReportsAnAuthorizationServerWithoutADataAddress(t *testing.T) {
	base := mcsdServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(organizationBundleWith([]string{"Endpoint/zb-oauth"}, authServerEndpoint)))
	})

	source, err := mcsdResolveFunc(base)(t.Context(), "00000020")

	require.NoError(t, err, "the organization was found; what it publishes is the answer")
	require.Empty(t, source.Address)
	require.Equal(t, "http://localhost:8080/nuts/oauth2/00000020", source.AuthorizationServer,
		"and the half it does publish has to come back, or an empty answer would satisfy this too")
}
