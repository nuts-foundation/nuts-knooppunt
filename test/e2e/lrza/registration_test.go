package lrza

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/profile"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Test_LRZARegistration drives the registration proxy (the write path to the LRZA) against the
// seeded Central Addressing Directory, which stands in for the LRZA. Like Test_LRZASync, the steps
// build on each other's state, so the test stops at the first failing step.
func Test_LRZARegistration(t *testing.T) {
	h := harness.Start(t)
	source := fhirclient.New(h.LRZaFHIRBaseURL, http.DefaultClient, nil)
	query := fhirclient.New(h.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
	client := &registrationClient{t: t, h: h, tenant: coding.URANamingSystem + "|" + sunflower.URA}
	sunflowerOrganizationID := *lrza.CareHomeSunflower().Id

	var endpointID, locationID string
	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"create an Endpoint and a Location in one transaction", func(t *testing.T) {
			resp := client.do(http.MethodPost, "", "", map[string]any{
				"resourceType": "Bundle",
				"type":         "transaction",
				"entry": []any{
					map[string]any{
						"fullUrl": "urn:uuid:5d8a2f61-0c1e-4d0b-8a7e-0f1d3c4b5a69",
						"resource": map[string]any{
							"resourceType":   "Endpoint",
							"status":         "active",
							"connectionType": map[string]any{"system": coding.EndpointConnectionTypeSystem, "code": "hl7-fhir-rest"},
							"payloadType": []any{map[string]any{"coding": []any{map[string]any{
								"system": coding.CSDDataCategoriesSystem, "code": "Patient",
							}}}},
							"address": "https://zonnebloem.example.org/fhir/registration-e2e",
						},
						"request": map[string]any{"method": "POST", "url": "Endpoint"},
					},
					map[string]any{
						"resource": map[string]any{
							"resourceType": "Location",
							"status":       "active",
							"name":         "Zonnebloem, locatie Noord",
							"type": []any{map[string]any{"coding": []any{map[string]any{
								"system": "http://terminology.hl7.org/CodeSystem/v3-RoleCode", "code": "NCCF",
							}}}},
							"managingOrganization": map[string]any{"reference": "Organization/" + sunflowerOrganizationID},
							"endpoint":             []any{map[string]any{"reference": "urn:uuid:5d8a2f61-0c1e-4d0b-8a7e-0f1d3c4b5a69"}},
						},
						"request": map[string]any{"method": "POST", "url": "Location"},
					},
				},
			})
			require.Equal(t, http.StatusOK, resp.status, string(resp.body))

			var result fhir.Bundle
			require.NoError(t, json.Unmarshal(resp.body, &result))
			require.Len(t, result.Entry, 2)
			for _, entry := range result.Entry {
				require.NotNil(t, entry.Response)
				assert.Contains(t, entry.Response.Status, "201")
			}

			var endpoint fhir.Endpoint
			require.NoError(t, json.Unmarshal(result.Entry[0].Resource, &endpoint))
			endpointID = *endpoint.Id
			var stored fhir.Endpoint
			require.NoError(t, source.Read("Endpoint/"+endpointID, &stored))
			assert.Contains(t, stored.Meta.Profile, profile.CSDEndpoint)
			require.NotNil(t, stored.ManagingOrganization, "defaulted to the tenant's Organization")
			assert.Equal(t, "Organization/"+sunflowerOrganizationID, *stored.ManagingOrganization.Reference)
			assert.Contains(t, stored.PayloadMimeType, coding.FHIRR4JSONMimeType)

			var location fhir.Location
			require.NoError(t, json.Unmarshal(result.Entry[1].Resource, &location))
			locationID = *location.Id
			var storedLocation fhir.Location
			require.NoError(t, source.Read("Location/"+locationID, &storedLocation))
			assert.Contains(t, storedLocation.Meta.Profile, profile.CSDLocation)
			require.Len(t, storedLocation.Identifier, 1, "a custodian-assigned identifier is generated")
			assert.Equal(t, coding.RFC3986NamingSystem, *storedLocation.Identifier[0].System)
			assert.Equal(t, sunflower.URA, *storedLocation.Identifier[0].Assigner.Identifier.Value)
			require.Len(t, storedLocation.Endpoint, 1)
			assert.Equal(t, "Endpoint/"+endpointID, *storedLocation.Endpoint[0].Reference, "the in-bundle reference is resolved")
		}},
		{"link the Endpoint to the tenant's Organization", func(t *testing.T) {
			read := client.do(http.MethodGet, "Organization/"+sunflowerOrganizationID, "", nil)
			require.Equal(t, http.StatusOK, read.status, string(read.body))
			etag := read.header.Get("ETag")
			require.NotEmpty(t, etag)

			var organization map[string]any
			require.NoError(t, json.Unmarshal(read.body, &organization))
			organization["endpoint"] = append(organization["endpoint"].([]any), map[string]any{"reference": "Endpoint/" + endpointID})
			resp := client.do(http.MethodPut, "Organization/"+sunflowerOrganizationID, etag, organization)
			require.Equal(t, http.StatusOK, resp.status, string(resp.body))
			assert.NotEqual(t, etag, resp.header.Get("ETag"), "a new version")

			var stored fhir.Organization
			require.NoError(t, source.Read("Organization/"+sunflowerOrganizationID, &stored))
			var references []string
			for _, endpoint := range stored.Endpoint {
				references = append(references, *endpoint.Reference)
			}
			assert.Contains(t, references, "Endpoint/"+endpointID)
		}},
		{"writing another tenant's Organization is forbidden", func(t *testing.T) {
			care2Cure := lrza.Care2Cure()
			care2Cure.Name = new("Taken over")
			resp := client.do(http.MethodPut, "Organization/"+*care2Cure.Id, "", care2Cure)
			require.Equal(t, http.StatusForbidden, resp.status, string(resp.body))

			var stored fhir.Organization
			require.NoError(t, source.Read("Organization/"+*care2Cure.Id, &stored))
			assert.Equal(t, *lrza.Care2Cure().Name, *stored.Name)
		}},
		{"an update of a stale version is rejected", func(t *testing.T) {
			read := client.do(http.MethodGet, "Location/"+locationID, "", nil)
			require.Equal(t, http.StatusOK, read.status, string(read.body))
			var location map[string]any
			require.NoError(t, json.Unmarshal(read.body, &location))
			location["name"] = "Zonnebloem, locatie Noord (renamed)"
			first := client.do(http.MethodPut, "Location/"+locationID, read.header.Get("ETag"), location)
			require.Equal(t, http.StatusOK, first.status, string(first.body))

			stale := client.do(http.MethodPut, "Location/"+locationID, read.header.Get("ETag"), location)
			assert.Equal(t, http.StatusPreconditionFailed, stale.status, string(stale.body))
		}},
		{"withdraw the Location", func(t *testing.T) {
			resp := client.do(http.MethodDelete, "Location/"+locationID, "", nil)
			require.Equal(t, http.StatusOK, resp.status, string(resp.body))

			var stored fhir.Location
			require.NoError(t, source.Read("Location/"+locationID, &stored))
			require.NotNil(t, stored.Status)
			assert.Equal(t, fhir.LocationStatusInactive, *stored.Status, "withdrawn by status, not deleted")

			// Every write requests a sync into the query directory, so the withdrawal shows up there
			// without a call to /lrza/update.
			require.Eventually(t, func() bool {
				local, ok := findLocalCopy[fhir.Location](t, h, query, "Location", locationID)
				return ok && local.Status != nil && *local.Status == fhir.LocationStatusInactive
			}, 30*time.Second, 500*time.Millisecond)
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}

// registrationClient calls the Knooppunt's /lrza/fhir operations for one tenant.
type registrationClient struct {
	t      *testing.T
	h      harness.Details
	tenant string
}

type registrationResponse struct {
	status int
	header http.Header
	body   []byte
}

func (c *registrationClient) do(method, path, ifMatch string, body any) registrationResponse {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(c.t, err)
		reader = bytes.NewReader(data)
	}
	target := c.h.KnooppuntInternalBaseURL.JoinPath("lrza", "fhir")
	if path != "" {
		target = target.JoinPath(path)
	}
	request, err := http.NewRequestWithContext(c.t.Context(), method, target.String(), reader)
	require.NoError(c.t, err)
	request.Header.Set("X-Tenant-ID", c.tenant)
	request.Header.Set("Content-Type", "application/fhir+json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response, err := http.DefaultClient.Do(request)
	require.NoError(c.t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(c.t, err)
	return registrationResponse{status: response.StatusCode, header: response.Header, body: data}
}

// findLocalCopy is readLocalCopy for a resource that may not have been synced yet.
func findLocalCopy[T any](t *testing.T, h harness.Details, query fhirclient.Client, resourceType, sourceID string) (T, bool) {
	t.Helper()
	var result T
	var bundle fhir.Bundle
	if err := query.Search(resourceType, map[string][]string{"_source": {sourceURL(h, resourceType, sourceID)}}, &bundle); err != nil || len(bundle.Entry) != 1 {
		return result, false
	}
	if err := json.Unmarshal(bundle.Entry[0].Resource, &result); err != nil {
		return result, false
	}
	return result, true
}
