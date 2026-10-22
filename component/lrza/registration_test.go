package lrza

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/profile"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tenants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sunflowerURA = "11111111"
	care2CureURA = "22222222"
	testUUID     = "00000000-0000-4000-8000-000000000001"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func uraIdentifierJSON(ura string) map[string]any {
	return toJSONMap(coding.URAIdentifier(ura, coding.DefaultURAAssignerKVK))
}

func organizationRef(id string) map[string]any {
	return map[string]any{"reference": "Organization/" + id, "type": "Organization"}
}

// testDirectory is the LRZA's content the tests start from: two care providers (sunflower, the
// tenant, with a department; care2cure, another tenant), each with a Location and an Endpoint.
func testDirectory() []resource {
	return []resource{
		{
			"resourceType": "Organization", "id": "sunflower", "name": "Sunflower",
			"identifier": []any{uraIdentifierJSON(sunflowerURA)},
			"type":       []any{map[string]any{"text": "care home"}},
		},
		{
			"resourceType": "Organization", "id": "sunflower-ward", "name": "Ward",
			"partOf": organizationRef("sunflower"),
		},
		{
			"resourceType": "Organization", "id": "care2cure", "name": "Care2Cure",
			"identifier": []any{uraIdentifierJSON(care2CureURA)},
		},
		testLocation("sunflower-location", "sunflower"),
		testLocation("care2cure-location", "care2cure"),
		testEndpoint("sunflower-endpoint", "sunflower"),
		testEndpoint("care2cure-endpoint", "care2cure"),
	}
}

func testLocation(id, organizationID string) resource {
	res := resource{
		"resourceType": "Location", "status": "active", "name": "Location " + id,
		"type":                 []any{map[string]any{"coding": []any{map[string]any{"system": "http://terminology.hl7.org/CodeSystem/v3-RoleCode", "code": "HOSP"}}}},
		"managingOrganization": organizationRef(organizationID),
	}
	if id != "" {
		res["id"] = id
	}
	return res
}

func testEndpoint(id, organizationID string) resource {
	res := resource{
		"resourceType":   "Endpoint",
		"status":         "active",
		"connectionType": map[string]any{"system": coding.EndpointConnectionTypeSystem, "code": "hl7-fhir-rest"},
		"payloadType":    []any{map[string]any{"coding": []any{map[string]any{"system": coding.CSDDataCategoriesSystem, "code": "Patient"}}}},
		"address":        "https://example.org/fhir",
	}
	if id != "" {
		res["id"] = id
	}
	if organizationID != "" {
		res["managingOrganization"] = organizationRef(organizationID)
	}
	return res
}

// newTestComponent creates a component with registration enabled against a fake LRZA, with a fast
// retry transport, a fixed clock and fixed generated UUIDs.
func newTestComponent(t *testing.T, lrza *fakeLRZA, configure ...func(*Config)) *Component {
	t.Helper()
	config := DefaultConfig()
	config.LRZABaseUrl = lrza.server.URL
	config.Registration.Enabled = true
	for _, fn := range configure {
		fn(&config)
	}
	c, err := New(config)
	require.NoError(t, err)
	baseURL, _ := url.Parse(lrza.server.URL)
	httpClient := &http.Client{Transport: &retryTransport{next: http.DefaultTransport, maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 10 * time.Millisecond}}
	c.fhirLRZAClient = fhirclient.New(baseURL, httpClient, &fhirclient.Config{UsePostSearch: false})
	c.registrar.client = c.fhirLRZAClient
	c.registrar.now = func() time.Time { return testNow }
	c.registrar.newUUID = func() string { return testUUID }
	return c
}

func tenant(ura string) tenants.ID {
	return tenants.ID{System: new(coding.URANamingSystem), Value: new(ura)}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

// response renders a strict-server response object as the HTTP response it writes.
type response struct {
	*httptest.ResponseRecorder
}

func (r response) resource(t *testing.T) resource {
	t.Helper()
	var res resource
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &res), r.Body.String())
	return res
}

func (r response) diagnostics(t *testing.T) []string {
	t.Helper()
	res := r.resource(t)
	require.Equal(t, "OperationOutcome", res.resourceType(), r.Body.String())
	var result []string
	for _, issue := range res["issue"].([]any) {
		diagnostics, _ := issue.(map[string]any)["diagnostics"].(string)
		result = append(result, diagnostics)
	}
	return result
}

func createResource(t *testing.T, c *Component, ura string, res resource) response {
	t.Helper()
	body := api.FHIRResource(mustJSON(t, res))
	result, err := c.CreateLrzaResource(context.Background(), api.CreateLrzaResourceRequestObject{
		ResourceType: api.CreateLrzaResourceParamsResourceType(res.resourceType()),
		Params:       api.CreateLrzaResourceParams{XTenantID: tenant(ura)},
		Body:         &body,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	require.NoError(t, result.VisitCreateLrzaResourceResponse(recorder))
	return response{recorder}
}

func updateResource(t *testing.T, c *Component, ura string, res resource, ifMatch *string) response {
	t.Helper()
	body := api.FHIRResource(mustJSON(t, res))
	result, err := c.UpdateLrzaResource(context.Background(), api.UpdateLrzaResourceRequestObject{
		ResourceType: api.UpdateLrzaResourceParamsResourceType(res.resourceType()),
		Id:           res.id(),
		Params:       api.UpdateLrzaResourceParams{XTenantID: tenant(ura), IfMatch: ifMatch},
		Body:         &body,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	require.NoError(t, result.VisitUpdateLrzaResourceResponse(recorder))
	return response{recorder}
}

func readResource(t *testing.T, c *Component, ura, resourceType, id string) response {
	t.Helper()
	result, err := c.ReadLrzaResource(context.Background(), api.ReadLrzaResourceRequestObject{
		ResourceType: api.ReadLrzaResourceParamsResourceType(resourceType),
		Id:           id,
		Params:       api.ReadLrzaResourceParams{XTenantID: tenant(ura)},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	require.NoError(t, result.VisitReadLrzaResourceResponse(recorder))
	return response{recorder}
}

func withdrawResource(t *testing.T, c *Component, ura, resourceType, id string) response {
	t.Helper()
	result, err := c.WithdrawLrzaResource(context.Background(), api.WithdrawLrzaResourceRequestObject{
		ResourceType: api.WithdrawLrzaResourceParamsResourceType(resourceType),
		Id:           id,
		Params:       api.WithdrawLrzaResourceParams{XTenantID: tenant(ura)},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	require.NoError(t, result.VisitWithdrawLrzaResourceResponse(recorder))
	return response{recorder}
}

func submitTransaction(t *testing.T, c *Component, ura string, bundle any) response {
	t.Helper()
	body := api.FHIRResource(mustJSON(t, bundle))
	result, err := c.SubmitLrzaTransaction(context.Background(), api.SubmitLrzaTransactionRequestObject{
		Params: api.SubmitLrzaTransactionParams{XTenantID: tenant(ura)},
		Body:   &body,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	require.NoError(t, result.VisitSubmitLrzaTransactionResponse(recorder))
	return response{recorder}
}

func TestRegistration_Ownership(t *testing.T) {
	withURA := func(ura string) resource {
		org := testDirectory()[0].clone()
		org["identifier"] = []any{uraIdentifierJSON(ura)}
		return org
	}
	withEndpoint := func(endpointID string) resource {
		org := testDirectory()[0].clone()
		org["endpoint"] = []any{map[string]any{"reference": "Endpoint/" + endpointID}}
		return org
	}
	for _, tc := range []struct {
		name     string
		update   bool
		res      resource
		status   int
		contains string
	}{
		{name: "Location of the tenant's own Organization", res: testLocation("", "sunflower"), status: http.StatusCreated},
		{name: "Location of a department, through partOf", res: testLocation("", "sunflower-ward"), status: http.StatusCreated},
		{name: "Location of another tenant's Organization", res: testLocation("", "care2cure"), status: http.StatusForbidden, contains: "URA 22222222"},
		{name: "Location of an Organization not in the LRZA", res: testLocation("", "unknown"), status: http.StatusForbidden, contains: "does not refer to an Organization in the LRZA"},
		{name: "moving another tenant's Location to the tenant", update: true, res: testLocation("care2cure-location", "sunflower"), status: http.StatusForbidden, contains: "current version in the LRZA does not belong to the tenant"},
		{name: "moving the tenant's Location to another tenant", update: true, res: testLocation("sunflower-location", "care2cure"), status: http.StatusForbidden, contains: "Location does not belong to the tenant"},
		{name: "linking the tenant's own Endpoint", update: true, res: withEndpoint("sunflower-endpoint"), status: http.StatusOK},
		{name: "linking another tenant's Endpoint", update: true, res: withEndpoint("care2cure-endpoint"), status: http.StatusForbidden, contains: "Endpoint the tenant does not manage"},
		{name: "changing the URA of the tenant's Organization", update: true, res: withURA("33333333"), status: http.StatusForbidden, contains: "URA 33333333"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lrza := newFakeLRZA(t, testDirectory()...)
			c := newTestComponent(t, lrza)

			var resp response
			if tc.update {
				resp = updateResource(t, c, sunflowerURA, tc.res, nil)
			} else {
				resp = createResource(t, c, sunflowerURA, tc.res)
			}

			require.Equal(t, tc.status, resp.Code, resp.Body.String())
			if tc.contains != "" {
				assert.Contains(t, resp.diagnostics(t)[0], tc.contains)
			}
			if tc.status == http.StatusForbidden {
				assert.Empty(t, lrza.writes(), "a rejected write must not reach the LRZA")
			}
		})
	}

	t.Run("rejected resource types", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)
		for _, resourceType := range []string{"OrganizationAffiliation", "Provenance", "Practitioner", "PractitionerRole"} {
			resp := createResource(t, c, sunflowerURA, resource{"resourceType": resourceType})
			assert.Equal(t, http.StatusForbidden, resp.Code, resourceType)
		}
		resp := createResource(t, c, sunflowerURA, resource{"resourceType": "Patient"})
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Empty(t, lrza.writes())
	})

	t.Run("read", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		own := readResource(t, c, sunflowerURA, "Location", "sunflower-location")
		require.Equal(t, http.StatusOK, own.Code, own.Body.String())
		assert.Equal(t, `W/"1"`, own.Header().Get("ETag"))
		assert.Equal(t, "sunflower-location", own.resource(t).id())

		foreign := readResource(t, c, sunflowerURA, "Location", "care2cure-location")
		assert.Equal(t, http.StatusForbidden, foreign.Code)

		missing := readResource(t, c, sunflowerURA, "Location", "missing")
		assert.Equal(t, http.StatusNotFound, missing.Code)
	})
}

func TestRegistration_Enrichment(t *testing.T) {
	t.Run("Location gets the profile and a generated AssignedId", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		resp := createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
		stored := lrza.get(resp.resource(t).reference())
		assert.Equal(t, []any{profile.CSDLocation}, stored["meta"].(map[string]any)["profile"])
		assert.Equal(t, []any{toJSONMap(map[string]any{
			"use":      "official",
			"system":   coding.RFC3986NamingSystem,
			"value":    "urn:uuid:" + testUUID,
			"assigner": coding.Custodian(coding.URAOIDNamingSystem, sunflowerURA),
		})}, stored["identifier"])
		assert.Equal(t, `W/"1"`, resp.Header().Get("ETag"))
		assert.Equal(t, "/lrza/fhir/"+stored.reference()+"/_history/1", resp.Header().Get("Location"))
	})

	t.Run("an update without the AssignedId keeps the current one", func(t *testing.T) {
		current := testLocation("sunflower-location", "sunflower")
		assignedID := map[string]any{"system": "http://example.org/locations", "value": "loc-1", "assigner": toJSONMap(coding.Custodian(coding.URAOIDNamingSystem, sunflowerURA))}
		current["identifier"] = []any{assignedID}
		directory := append(testDirectory()[:3], current)
		lrza := newFakeLRZA(t, directory...)
		c := newTestComponent(t, lrza)

		update := testLocation("sunflower-location", "sunflower")
		update["name"] = "Renamed"
		resp := updateResource(t, c, sunflowerURA, update, nil)

		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		stored := lrza.get("Location/sunflower-location")
		assert.Equal(t, "Renamed", stored["name"])
		assert.Equal(t, []any{assignedID}, stored["identifier"])
	})

	t.Run("a legacy URA identifier is moved to the OID system", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, func(config *Config) {
			config.Registration.URAAssignerKVK = "12345678"
		})

		org := testDirectory()[0].clone()
		org["identifier"] = []any{map[string]any{"system": coding.URANamingSystem, "value": sunflowerURA}}
		resp := updateResource(t, c, sunflowerURA, org, nil)

		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		stored := lrza.get("Organization/sunflower")
		assert.Equal(t, []any{toJSONMap(coding.URAIdentifier(sunflowerURA, "12345678"))}, stored["identifier"])
	})

	t.Run("Endpoint gets the tenant's Organization and the FHIR version", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		endpoint := testEndpoint("", "")
		endpoint["meta"] = map[string]any{"profile": []any{"http://example.org/own-profile"}}
		resp := createResource(t, c, sunflowerURA, endpoint)

		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
		stored := lrza.get(resp.resource(t).reference())
		assert.Equal(t, organizationRef("sunflower"), stored["managingOrganization"])
		assert.Equal(t, []any{coding.FHIRR4JSONMimeType}, stored["payloadMimeType"])
		assert.Equal(t, []any{"http://example.org/own-profile", profile.CSDEndpoint}, stored["meta"].(map[string]any)["profile"])
	})

	t.Run("fields the client set are kept", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		endpoint := testEndpoint("", "sunflower-ward")
		endpoint["payloadMimeType"] = []any{"application/fhir+json; fhirVersion=4.0.1"}
		resp := createResource(t, c, sunflowerURA, endpoint)

		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
		stored := lrza.get(resp.resource(t).reference())
		assert.Equal(t, organizationRef("sunflower-ward"), stored["managingOrganization"])
		assert.Equal(t, []any{"application/fhir+json; fhirVersion=4.0.1"}, stored["payloadMimeType"])
	})

	t.Run("an Endpoint needs the tenant's Organization in the LRZA", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		resp := createResource(t, c, "99999999", testEndpoint("", ""))

		require.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
		assert.Contains(t, resp.diagnostics(t)[0], "is not registered in the LRZA")
	})
}

func TestRegistration_Validation(t *testing.T) {
	t.Run("returns every problem as an issue, without writing", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		location := testLocation("", "sunflower")
		delete(location, "name")
		delete(location, "type")
		resp := createResource(t, c, sunflowerURA, location)

		require.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
		outcome := resp.resource(t)
		var expressions []any
		for _, issue := range outcome["issue"].([]any) {
			expressions = append(expressions, issue.(map[string]any)["expression"].([]any)...)
		}
		assert.ElementsMatch(t, []any{"Location.name", "Location.type"}, expressions)
		assert.Empty(t, lrza.writes())
	})
}

func TestRegistration_IfMatch(t *testing.T) {
	t.Run("the version read for the ownership check is sent when the client sends none", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		resp := updateResource(t, c, sunflowerURA, testLocation("sunflower-location", "sunflower"), nil)

		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		puts := lrza.requestsTo(http.MethodPut, "/Location/sunflower-location")
		require.Len(t, puts, 1)
		assert.Equal(t, `W/"1"`, puts[0].header.Get("If-Match"))
		assert.Equal(t, "return=representation", puts[0].header.Get("Prefer"))
		assert.Equal(t, `W/"2"`, resp.Header().Get("ETag"))
	})

	t.Run("the client's If-Match is forwarded unchanged, and a 412 passed through", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		resp := updateResource(t, c, sunflowerURA, testLocation("sunflower-location", "sunflower"), new(`W/"7"`))

		require.Equal(t, http.StatusPreconditionFailed, resp.Code, resp.Body.String())
		assert.Equal(t, []string{`version conflict: W/"7"`}, resp.diagnostics(t), "the LRZA's OperationOutcome is passed through")
		puts := lrza.requestsTo(http.MethodPut, "/Location/sunflower-location")
		require.Len(t, puts, 1)
		assert.Equal(t, `W/"7"`, puts[0].header.Get("If-Match"))
	})

	t.Run("a 429 is passed through with its Retry-After once retries are used up", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)
		lrza.failNext(http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests)

		resp := createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		require.Equal(t, http.StatusTooManyRequests, resp.Code, resp.Body.String())
		assert.Equal(t, "0", resp.Header().Get("Retry-After"))
	})
}

func TestRegistration_Withdraw(t *testing.T) {
	for _, tc := range []struct {
		resourceType string
		id           string
		check        func(t *testing.T, stored resource)
	}{
		{"Organization", "sunflower-ward", func(t *testing.T, stored resource) { assert.Equal(t, false, stored["active"]) }},
		{"Location", "sunflower-location", func(t *testing.T, stored resource) { assert.Equal(t, "inactive", stored["status"]) }},
		{"Endpoint", "sunflower-endpoint", func(t *testing.T, stored resource) {
			assert.Equal(t, "off", stored["status"])
			assert.Equal(t, "2026-10-20T12:00:00Z", stored["period"].(map[string]any)["end"])
		}},
	} {
		t.Run(tc.resourceType, func(t *testing.T) {
			lrza := newFakeLRZA(t, testDirectory()...)
			c := newTestComponent(t, lrza)

			resp := withdrawResource(t, c, sunflowerURA, tc.resourceType, tc.id)

			require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
			stored := lrza.get(tc.resourceType + "/" + tc.id)
			assert.Equal(t, "2", stored.versionID(), "withdrawn by an update, not deleted")
			tc.check(t, stored)
			puts := lrza.requestsTo(http.MethodPut, "/"+tc.resourceType+"/"+tc.id)
			require.Len(t, puts, 1)
			assert.Equal(t, `W/"1"`, puts[0].header.Get("If-Match"))
		})
	}

	t.Run("HealthcareService", func(t *testing.T) {
		res := resource{"resourceType": "HealthcareService", "active": true}
		require.NoError(t, withdraw(res, testNow))
		assert.Equal(t, false, res["active"])
	})

	t.Run("another tenant's resource", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		resp := withdrawResource(t, c, sunflowerURA, "Endpoint", "care2cure-endpoint")

		require.Equal(t, http.StatusForbidden, resp.Code)
		assert.Empty(t, lrza.writes())
	})
}

func TestRegistration_Mandate(t *testing.T) {
	const code = "http://example.org/affiliation-types|data-source"
	const vendor = "http://kvk.nl|87654321"
	configure := func(config *Config) {
		config.Registration.Mandate = MandateConfig{Code: code, Vendor: vendor}
	}
	affiliation := resource{
		"resourceType": "OrganizationAffiliation", "id": "mandate", "active": true,
		"organization":              organizationRef("sunflower"),
		"participatingOrganization": map[string]any{"identifier": map[string]any{"system": "http://kvk.nl", "value": "87654321"}},
		"code":                      []any{map[string]any{"coding": []any{map[string]any{"system": "http://example.org/affiliation-types", "code": "data-source"}}}},
	}

	t.Run("granted", func(t *testing.T) {
		lrza := newFakeLRZA(t, append(testDirectory(), affiliation)...)
		c := newTestComponent(t, lrza, configure)

		resp := createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		assert.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	})

	t.Run("not granted", func(t *testing.T) {
		inactive := affiliation.clone()
		inactive["active"] = false
		lrza := newFakeLRZA(t, append(testDirectory(), inactive)...)
		c := newTestComponent(t, lrza, configure)

		resp := createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		require.Equal(t, http.StatusForbidden, resp.Code, resp.Body.String())
		assert.Contains(t, resp.diagnostics(t)[0], "has not granted the mandate")
		assert.Empty(t, lrza.writes())
	})

	t.Run("invalid configuration", func(t *testing.T) {
		config := DefaultConfig()
		config.Registration.Enabled = true
		config.Registration.Mandate = MandateConfig{Code: code}
		_, err := New(config)
		assert.ErrorContains(t, err, "mandate.vendor")
	})
}

func TestRegistration_Transaction(t *testing.T) {
	t.Run("entries referring to each other, and a delete rewritten to a withdrawal", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		service := resource{
			"resourceType": "HealthcareService", "name": "Cardiology",
			"type":       []any{map[string]any{"text": "cardiology"}},
			"providedBy": organizationRef("sunflower"),
			"endpoint":   []any{map[string]any{"reference": "urn:uuid:new-endpoint"}},
		}
		bundle := map[string]any{
			"resourceType": "Bundle", "type": "transaction",
			"entry": []any{
				map[string]any{"fullUrl": "urn:uuid:new-endpoint", "resource": testEndpoint("", ""), "request": map[string]any{"method": "POST", "url": "Endpoint"}},
				map[string]any{"fullUrl": "urn:uuid:new-service", "resource": service, "request": map[string]any{"method": "POST", "url": "HealthcareService"}},
				map[string]any{"request": map[string]any{"method": "DELETE", "url": "Location/sunflower-location"}},
			},
		}

		resp := submitTransaction(t, c, sunflowerURA, bundle)

		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		result := resp.resource(t)
		assert.Equal(t, "transaction-response", result["type"])
		entries := result["entry"].([]any)
		require.Len(t, entries, 3)

		endpointRef := entries[0].(map[string]any)["resource"].(map[string]any)["id"].(string)
		assert.Equal(t, "Endpoint/"+endpointRef+"/_history/1", entries[0].(map[string]any)["response"].(map[string]any)["location"], "relative to the Knooppunt, not the LRZA")
		storedService := lrza.get(resource(entries[1].(map[string]any)["resource"].(map[string]any)).reference())
		assert.Equal(t, []any{map[string]any{"reference": "Endpoint/" + endpointRef}}, storedService["endpoint"])
		assert.NotEmpty(t, storedService["identifier"], "AssignedId generated")
		storedEndpoint := lrza.get("Endpoint/" + endpointRef)
		assert.Equal(t, organizationRef("sunflower"), storedEndpoint["managingOrganization"])
		assert.Equal(t, "inactive", lrza.get("Location/sunflower-location")["status"])

		var sent struct {
			Entry []struct {
				Request map[string]any `json:"request"`
			} `json:"entry"`
		}
		transactions := lrza.requestsTo(http.MethodPost, "/")
		require.Len(t, transactions, 1)
		require.NoError(t, json.Unmarshal(transactions[0].body, &sent))
		assert.Equal(t, map[string]any{"method": "PUT", "url": "Location/sunflower-location", "ifMatch": `W/"1"`}, sent.Entry[2].Request)
	})

	t.Run("an entry for another tenant rejects the whole transaction", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		bundle := map[string]any{
			"resourceType": "Bundle", "type": "transaction",
			"entry": []any{
				map[string]any{"resource": testLocation("", "sunflower"), "request": map[string]any{"method": "POST", "url": "Location"}},
				map[string]any{"resource": testLocation("care2cure-location", "care2cure"), "request": map[string]any{"method": "PUT", "url": "Location/care2cure-location"}},
			},
		}

		resp := submitTransaction(t, c, sunflowerURA, bundle)

		require.Equal(t, http.StatusForbidden, resp.Code, resp.Body.String())
		assert.Contains(t, resp.diagnostics(t)[0], "entry[1]")
		assert.Empty(t, lrza.writes())
	})

	t.Run("a fullUrl can't stand in for another tenant's resource", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		// The Organization entry claims the fullUrl of care2cure's Organization; the LRZA would
		// resolve the Location's reference to the real one.
		ownOrganization := testDirectory()[0].clone()
		delete(ownOrganization, "id")
		bundle := map[string]any{
			"resourceType": "Bundle", "type": "transaction",
			"entry": []any{
				map[string]any{"fullUrl": "Organization/care2cure", "resource": ownOrganization, "request": map[string]any{"method": "POST", "url": "Organization"}},
				map[string]any{"resource": testLocation("", "care2cure"), "request": map[string]any{"method": "POST", "url": "Location"}},
			},
		}

		resp := submitTransaction(t, c, sunflowerURA, bundle)

		require.Equal(t, http.StatusForbidden, resp.Code, resp.Body.String())
		assert.Empty(t, lrza.writes())
	})

	t.Run("validation issues point at their entry", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		location := testLocation("", "sunflower")
		delete(location, "name")
		bundle := map[string]any{
			"resourceType": "Bundle", "type": "transaction",
			"entry": []any{map[string]any{"resource": location, "request": map[string]any{"method": "POST", "url": "Location"}}},
		}

		resp := submitTransaction(t, c, sunflowerURA, bundle)

		require.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
		issue := resp.resource(t)["issue"].([]any)[0].(map[string]any)
		assert.Equal(t, []any{"Bundle.entry[0].resource.name"}, issue["expression"])
	})

	t.Run("not a transaction", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		resp := submitTransaction(t, c, sunflowerURA, map[string]any{"resourceType": "Bundle", "type": "batch", "entry": []any{}})

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestRegistration_SyncAfterWrite(t *testing.T) {
	t.Run("a burst of writes collapses into one pending sync", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, func(config *Config) {
			config.QueryBaseUrl = "http://query.example/fhir"
		})

		createResource(t, c, sunflowerURA, testLocation("", "sunflower"))
		createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		assert.Len(t, c.syncRequests, 1)
	})

	t.Run("no sync without a query directory", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza)

		createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		assert.Empty(t, c.syncRequests)
	})

	t.Run("no sync when disabled", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, func(config *Config) {
			config.QueryBaseUrl = "http://query.example/fhir"
			config.Registration.SyncAfterWrite = false
		})

		createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		assert.Empty(t, c.syncRequests)
	})

	t.Run("a requested sync runs, and Stop ends the goroutine", func(t *testing.T) {
		var syncs int
		query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"resourceType": "Bundle", "type": "transaction-response"})
		}))
		defer query.Close()
		lrza := newFakeLRZA(t)
		c := newTestComponent(t, lrza, func(config *Config) {
			config.QueryBaseUrl = query.URL
		})
		require.NoError(t, c.Start())

		c.requestSync()

		require.Eventually(t, func() bool {
			syncs = len(lrza.requestsTo(http.MethodGet, "/OrganizationAffiliation"))
			return syncs > 0
		}, 5*time.Second, 10*time.Millisecond, "the sync reads every resource type, the last being OrganizationAffiliation")
		require.NoError(t, c.Stop(context.Background()))
	})
}

func TestRelativeLocation(t *testing.T) {
	stored := []byte(`{"resourceType":"Endpoint","id":"123","meta":{"versionId":"2"}}`)
	tests := []struct {
		name         string
		stored       []byte
		lrzaLocation string
		expected     string
	}{
		{name: "from the stored resource", stored: stored, lrzaLocation: "https://lrza.example.org/fhir/Endpoint/other/_history/9", expected: "Endpoint/123/_history/2"},
		{name: "stored resource without versionId", stored: []byte(`{"resourceType":"Endpoint","id":"123"}`), expected: "Endpoint/123"},
		{name: "absolute LRZA location", lrzaLocation: "https://lrza.example.org/fhir/Endpoint/123/_history/2", expected: "Endpoint/123/_history/2"},
		{name: "absolute LRZA location without version", lrzaLocation: "https://lrza.example.org/fhir/Endpoint/123", expected: "Endpoint/123"},
		{name: "relative LRZA location", lrzaLocation: "Endpoint/123/_history/2", expected: "Endpoint/123/_history/2"},
		{name: "neither", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, relativeLocation(tt.stored, tt.lrzaLocation))
		})
	}
}

func TestVersionConflictStatus(t *testing.T) {
	assert.Equal(t, http.StatusPreconditionFailed, versionConflictStatus(http.StatusConflict, true), "a 409 on a versioned update is a stale version")
	assert.Equal(t, http.StatusConflict, versionConflictStatus(http.StatusConflict, false))
	assert.Equal(t, http.StatusPreconditionFailed, versionConflictStatus(http.StatusPreconditionFailed, true))
	assert.Equal(t, http.StatusUnprocessableEntity, versionConflictStatus(http.StatusUnprocessableEntity, true))
}
