package mcsd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
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

		t.Run("assert resource sync'd from LRZa Admin Directory", func(t *testing.T) {
			// This is the root/discovery directory, so only mCSD Directory endpoints should be present
			var response mcsd.UpdateReport
			require.NoError(t, json.Unmarshal(responseData, &response))
			assert.Equalf(t, 2, mapEntrySuffix(response, "lrza-mcsd-admin").CountCreated, "created=2 in %v", response)
		})

		queryFHIRClient := fhirclient.New(harnessDetail.MCSDCacheFHIRBaseURL, http.DefaultClient, nil)
		t.Run("assert Sunflower organization resources", func(t *testing.T) {
			org, err := searchOrg(queryFHIRClient, harnessDetail.SunflowerURA)
			require.NoError(t, err)
			assert.Equal(t, "Sunflower Care Home", *org.Name)

			// Assert mCSD-directory endpoint exists in query directory (from root directory)
			mcsdEndpoint, err := searchEndpoint(queryFHIRClient, "mcsd-directory", harnessDetail.SunflowerURA)
			require.NoError(t, err)
			require.NotNil(t, mcsdEndpoint, "mCSD-directory endpoint should exist for Sunflower")
			assert.Equal(t, "https://example.com/sunflower/mcsd", mcsdEndpoint.Address)

			// Assert FHIR endpoint exists in query directory (from admin directory)
			fhirEndpoints, err := searchAllEndpoints(queryFHIRClient, "fhir")
			require.NoError(t, err)
			sunflowerFHIREndpoint := findEndpointByAddress(fhirEndpoints, "https://example.com/sunflower/fhir")
			require.NotNil(t, sunflowerFHIREndpoint, "FHIR endpoint should exist for Sunflower")
			assert.Equal(t, "https://example.com/sunflower/fhir", sunflowerFHIREndpoint.Address)
		})
		t.Run("assert Care2Cure organization resources", func(t *testing.T) {
			org, err := searchOrg(queryFHIRClient, harnessDetail.Care2CureURA)
			require.NoError(t, err)
			assert.Equal(t, "Care2Cure Hospital", *org.Name)

			// Assert mCSD-directory endpoint exists in query directory (from root directory)
			mcsdEndpoint, err := searchEndpoint(queryFHIRClient, "mcsd-directory", harnessDetail.Care2CureURA)
			require.NoError(t, err)
			require.NotNil(t, mcsdEndpoint, "mCSD-directory endpoint should exist for Care2Cure")
			assert.Equal(t, "https://example.com/care2curehospital/mcsd", mcsdEndpoint.Address)

			// Assert FHIR endpoint exists in query directory (from admin directory)
			fhirEndpoints, err := searchAllEndpoints(queryFHIRClient, "fhir")
			require.NoError(t, err)
			care2cureFHIREndpoint := findEndpointByAddress(fhirEndpoints, "https://example.com/care2curehospital/fhir")
			require.NotNil(t, care2cureFHIREndpoint, "FHIR endpoint should exist for Care2Cure")
			assert.Equal(t, "https://example.com/care2curehospital/fhir", care2cureFHIREndpoint.Address)
		})
	})
}

func searchOrg(client fhirclient.Client, ura string) (*fhir.Organization, error) {
	var searchResult fhir.Bundle
	err := client.Search("Organization", url.Values{"identifier": []string{coding.URANamingSystem + "|" + ura}}, &searchResult)
	if err != nil {
		return nil, err
	}
	if len(searchResult.Entry) == 0 {
		return nil, nil
	}
	var organization fhir.Organization
	if err := json.Unmarshal(searchResult.Entry[0].Resource, &organization); err != nil {
		return nil, err
	}
	return &organization, nil
}

func searchEndpoint(client fhirclient.Client, connectionType string, organizationURA string) (*fhir.Endpoint, error) {
	// First find the organization to get its endpoint references
	org, err := searchOrg(client, organizationURA)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, nil
	}

	// Check if organization has endpoint references
	if org.Endpoint == nil || len(org.Endpoint) == 0 {
		return nil, nil
	}

	// For each endpoint reference in the organization, fetch and check the connection type
	for _, endpointRef := range org.Endpoint {
		if endpointRef.Reference == nil {
			continue
		}

		// Extract endpoint ID from reference (assuming format "Endpoint/id")
		refParts := strings.Split(*endpointRef.Reference, "/")
		if len(refParts) != 2 || refParts[0] != "Endpoint" {
			continue
		}
		endpointID := refParts[1]

		// Fetch the endpoint by ID
		var endpoint fhir.Endpoint
		err = client.Read("Endpoint/"+endpointID, &endpoint)
		if err != nil {
			continue // Skip if endpoint not found or error
		}

		// Check if this endpoint has the required connection type
		if endpoint.ConnectionType.Code != nil && *endpoint.ConnectionType.Code == connectionType {
			return &endpoint, nil
		}
	}

	return nil, nil
}

func searchAllEndpoints(client fhirclient.Client, connectionType string) ([]fhir.Endpoint, error) {
	var allEndpoints []fhir.Endpoint

	// Perform a search for all endpoints with the given connection type
	var searchResult fhir.Bundle
	err := client.Search("Endpoint", url.Values{"connection-type": []string{connectionType}}, &searchResult)
	if err != nil {
		return nil, err
	}

	// Iterate through the search results and unmarshal each endpoint
	for _, entry := range searchResult.Entry {
		var endpoint fhir.Endpoint
		if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
			return nil, err
		}
		allEndpoints = append(allEndpoints, endpoint)
	}

	return allEndpoints, nil
}

func findEndpointByAddress(endpoints []fhir.Endpoint, address string) *fhir.Endpoint {
	for _, endpoint := range endpoints {
		if endpoint.Address == address {
			return &endpoint
		}
	}
	return nil
}

func mapEntrySuffix(r mcsd.UpdateReport, suffix string) mcsd.DirectoryUpdateReport {
	for key, value := range r {
		if strings.HasSuffix(key, suffix) {
			return value
		}
	}
	return mcsd.DirectoryUpdateReport{}
}
