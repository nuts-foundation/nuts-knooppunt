package lrza

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	lrzacomponent "github.com/nuts-foundation/nuts-knooppunt/component/lrza"
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
// seeded Central Addressing Directory, which stands in for the LRZA: the mock LRZA
// (mock-components/lrza) authorizes writes by the care provider's mandate, refuses a DELETE or an
// update without If-Match, validates against the IG's profiles and verifies Provenance signatures.
// Every write below therefore also proves the Knooppunt sends what the LRZA accepts. Like
// Test_LRZASync, the steps build on each other's state, so the test stops at the first failing step.
//
// The Knooppunt's own mandate check (lrza.registration.mandate) is off, so a missing mandate is the
// mock's 403, passed through.
func Test_LRZARegistration(t *testing.T) {
	h := harness.StartWithLRZASigning(t, map[string]lrzacomponent.SigningConfig{sunflower.URA: signingCertificate(t)})
	// Reads the directory to check what the Knooppunt wrote, as the operator: a read as anyone else
	// would take a failure armed for the Knooppunt.
	source := h.LRZAOperator
	query := fhirclient.New(h.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
	client := &registrationClient{t: t, h: h, tenant: coding.URANamingSystem + "|" + sunflower.URA}
	care2CureClient := &registrationClient{t: t, h: h, tenant: coding.URANamingSystem + "|" + lrza.Care2CureURA}
	sunflowerOrganizationID := *lrza.CareHomeSunflower().Id
	care2CureMandate := lrza.Mandate("5f0e7c3a-2b1d-4c9e-8a7f-1e2d3c4b5a6f", *lrza.Care2Cure().Id, lrza.Care2CureURA)

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
		{"rate limiting is waited out", func(t *testing.T) {
			read := client.do(http.MethodGet, "Location/"+locationID, "", nil)
			require.Equal(t, http.StatusOK, read.status, string(read.body))
			var location map[string]any
			require.NoError(t, json.Unmarshal(read.body, &location))
			location["name"] = "Zonnebloem, locatie Noord (rate limited)"

			armFailure(t, h, http.StatusTooManyRequests, 2, 1, http.MethodPut)
			resp := client.do(http.MethodPut, "Location/"+locationID, read.header.Get("ETag"), location)

			require.Equal(t, http.StatusOK, resp.status, string(resp.body))
			assert.Equal(t, 0, armedFailures(t, h), "both 429s were answered, and retried")
		}},
		{"a create the LRZA failed is not retried", func(t *testing.T) {
			armFailure(t, h, http.StatusServiceUnavailable, 1, 0, http.MethodPost)
			resp := client.do(http.MethodPost, "Endpoint", "", map[string]any{
				"resourceType":   "Endpoint",
				"status":         "active",
				"connectionType": map[string]any{"system": coding.EndpointConnectionTypeSystem, "code": "hl7-fhir-rest"},
				"payloadType": []any{map[string]any{"coding": []any{map[string]any{
					"system": coding.CSDDataCategoriesSystem, "code": "Patient",
				}}}},
				"address": "https://zonnebloem.example.org/fhir/not-retried",
			})

			// A retry would have succeeded: only one failure was armed. A POST may have been applied
			// before a 5xx, so the Knooppunt leaves retrying it to the client.
			assert.Equal(t, http.StatusServiceUnavailable, resp.status, string(resp.body))
			assert.Equal(t, 0, armedFailures(t, h))
		}},
		{"a resource that passes local validation but not the profile is rejected by the LRZA", func(t *testing.T) {
			resp := client.do(http.MethodPost, "Endpoint", "", map[string]any{
				"resourceType":   "Endpoint",
				"status":         "active",
				"connectionType": map[string]any{"system": coding.EndpointConnectionTypeSystem, "code": "hl7-fhir-rest"},
				// The right code system, which is all the Knooppunt checks, but no code of it.
				"payloadType": []any{map[string]any{"coding": []any{map[string]any{
					"system": coding.CSDDataCategoriesSystem, "code": "NoSuchCategory",
				}}}},
				"address": "https://zonnebloem.example.org/fhir/invalid",
			})

			require.Equal(t, http.StatusUnprocessableEntity, resp.status, string(resp.body))
			assert.Contains(t, string(resp.body), "nl-gf-payloadtype-req", "the LRZA's OperationOutcome is passed through")
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
		{"every write was signed, and the LRZA verified the signatures", func(t *testing.T) {
			// The LRZA only stores a Provenance whose signature verifies, so finding one per written
			// resource proves both.
			for _, target := range []string{"Endpoint/" + endpointID, "Location/" + locationID} {
				var provenances fhir.Bundle
				require.NoError(t, source.Search("Provenance", url.Values{"target": {target}}, &provenances))
				assert.NotEmpty(t, provenances.Entry, "a Provenance for %s", target)
			}
		}},
		{"the LRZA rejects a Provenance whose signature does not verify", func(t *testing.T) {
			// The Endpoint's: it hasn't changed since it was signed, so its Provenance verifies as is.
			var provenances fhir.Bundle
			require.NoError(t, source.Search("Provenance", url.Values{"target": {"Endpoint/" + endpointID}}, &provenances))
			require.NotEmpty(t, provenances.Entry)
			var provenance map[string]any
			require.NoError(t, json.Unmarshal(provenances.Entry[0].Resource, &provenance))
			delete(provenance, "id")
			delete(provenance, "meta")

			resent := lrzaDirect(t, h, http.MethodPost, "Provenance", "", "", provenance)
			require.Equal(t, http.StatusCreated, resent.status, "the Provenance as signed: %s", resent.body)

			signature := provenance["signature"].([]any)[0].(map[string]any)
			compact, err := base64.StdEncoding.DecodeString(signature["data"].(string))
			require.NoError(t, err)
			// Another signature value: a character from its middle, as the last also carries padding.
			at := len(compact) - 20
			compact[at] = map[bool]byte{true: 'B', false: 'A'}[compact[at] == 'A']
			signature["data"] = base64.StdEncoding.EncodeToString(compact)

			resp := lrzaDirect(t, h, http.MethodPost, "Provenance", "", "", provenance)

			require.Equal(t, http.StatusUnprocessableEntity, resp.status, string(resp.body))
			assert.Contains(t, string(resp.body), "does not match")
		}},
		{"a care provider that granted no mandate is refused by the LRZA", func(t *testing.T) {
			resp := care2CureClient.do(http.MethodPost, "Endpoint", "", care2CureEndpoint())

			require.Equal(t, http.StatusForbidden, resp.status, string(resp.body))
			assert.Contains(t, string(resp.body), "has not granted", "the LRZA's OperationOutcome is passed through")
		}},
		{"once the care provider grants the mandate, the write succeeds", func(t *testing.T) {
			grant := lrzaDirect(t, h, http.MethodPost, "OrganizationAffiliation", lrza.CareProviderCaller(lrza.Care2CureURA), "", care2CureMandate)
			require.Equal(t, http.StatusCreated, grant.status, string(grant.body))
			var granted fhir.OrganizationAffiliation
			require.NoError(t, json.Unmarshal(grant.body, &granted))
			care2CureMandate.Id = granted.Id

			resp := care2CureClient.do(http.MethodPost, "Endpoint", "", care2CureEndpoint())
			assert.Equal(t, http.StatusCreated, resp.status, string(resp.body))
		}},
		{"once the care provider revokes the mandate, the write is refused again", func(t *testing.T) {
			var current fhir.OrganizationAffiliation
			var headers fhirclient.Headers
			require.NoError(t, source.Read("OrganizationAffiliation/"+*care2CureMandate.Id, &current, fhirclient.ResponseHeaders(&headers)))
			current.Active = new(false)
			revoke := lrzaDirect(t, h, http.MethodPut, "OrganizationAffiliation/"+*current.Id,
				lrza.CareProviderCaller(lrza.Care2CureURA), headers.Get("ETag"), current)
			require.Equal(t, http.StatusOK, revoke.status, string(revoke.body))

			resp := care2CureClient.do(http.MethodPost, "Endpoint", "", care2CureEndpoint())
			assert.Equal(t, http.StatusForbidden, resp.status, string(resp.body))
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}

// care2CureEndpoint is an Endpoint Care2Cure registers, for the mandate steps.
func care2CureEndpoint() map[string]any {
	return map[string]any{
		"resourceType":   "Endpoint",
		"status":         "active",
		"connectionType": map[string]any{"system": coding.EndpointConnectionTypeSystem, "code": "hl7-fhir-rest"},
		"payloadType": []any{map[string]any{"coding": []any{map[string]any{
			"system": coding.CSDDataCategoriesSystem, "code": "CareServiceEntities",
		}}}},
		"address": "https://example.com/care2curehospital/fhir/registration-e2e",
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
	// HAPI caches search results (for 60s by default), so a poll that starts before the sync has written
	// the copy would keep getting the cached empty result: ask for a fresh one.
	noCache := fhirclient.RequestHeaders(http.Header{"Cache-Control": []string{"no-cache"}})
	if err := query.Search(resourceType, map[string][]string{"_source": {sourceURL(h, resourceType, sourceID)}}, &bundle, noCache); err != nil || len(bundle.Entry) != 1 {
		return result, false
	}
	if err := json.Unmarshal(bundle.Entry[0].Resource, &result); err != nil {
		return result, false
	}
	return result, true
}
