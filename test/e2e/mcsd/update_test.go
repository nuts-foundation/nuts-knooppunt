package mcsd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
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
		t.Log(string(responseData))

		t.Run("assert resource sync'd from LRZa Admin Directory", func(t *testing.T) {
			// This is the root/discovery directory, so only mCSD Directory endpoints should be present
			var response mcsd.UpdateReport
			require.NoError(t, json.Unmarshal(responseData, &response))
			assert.Equalf(t, 2, mapEntrySuffix(response, "lrza-mcsd-admin").CountCreated, "created=2 in %v", response)
		})

		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
		t.Run("assert Sunflower organization resources", func(t *testing.T) {
			expectedOrg := lrza.Care2Cure()
			org, err := searchOrg(queryFHIRClient, harnessDetail.SunflowerURA)
			require.NoError(t, err)
			assert.Equal(t, "Sunflower Care Home", *org.Name)
			assert.NotEqual(t, *expectedOrg.Id, *org.Id, "copy of organization in local Query Directory should have new ID")
			// TODO: for some reason, meta is not populated correctly, needs further investigation
			//assert.Equal(t, "the-source", *org.Meta.Source, "copy of organization in local Query Directory should have new Meta.Source")

			// Assert mCSD-directory endpoint exists in query directory (from root directory)
			// TODO: Not possible yet, since the mCSD Directory endpoints comes from the root directory,
			//       but the Organization resource from the org directory, which doesn't reference its mCSD Directory.
			// assertEndpoint(t, queryFHIRClient, harnessDetail.SunflowerURA, "mcsd-directory", "/sunflower/mcsd")

			// Assert FHIR endpoint exists in query directory (from admin directory)
			assertEndpoint(t, queryFHIRClient, harnessDetail.SunflowerURA, "fhir", "/sunflower/fhir")
		})
		t.Run("assert Care2Cure organization resources", func(t *testing.T) {
			expectedOrg := lrza.Care2Cure()
			org, err := searchOrg(queryFHIRClient, harnessDetail.Care2CureURA)
			require.NoError(t, err)
			assert.Equal(t, "Care2Cure Hospital", *org.Name)
			assert.NotEqual(t, *expectedOrg.Id, *org.Id, "copy of organization in local Query Directory should have new ID")
			// TODO: for some reason, meta is not populated correctly, needs further investigation
			//assert.Equal(t, "the-source", *org.Meta.Source, "copy of organization in local Query Directory should have new Meta.Source")

			// Assert mCSD-directory endpoint exists in query directory (from root directory)
			// TODO: Not possible yet, since the mCSD Directory endpoints comes from the root directory,
			//       but the Organization resource from the org directory, which doesn't reference its mCSD Directory.
			//assertEndpoint(t, queryFHIRClient, harnessDetail.Care2CureURA, "mcsd-directory", "/care2curehospital/mcsd")

			// Assert FHIR endpoint exists in query directory (from admin directory)
			assertEndpoint(t, queryFHIRClient, harnessDetail.Care2CureURA, "fhir", "/care2curehospital/fhir")
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
	} else if len(searchResult.Entry) > 1 {
		return nil, fmt.Errorf("expected 0..1 results, got %d", len(searchResult.Entry))
	}
	var organization fhir.Organization
	if err := json.Unmarshal(searchResult.Entry[0].Resource, &organization); err != nil {
		return nil, err
	}
	return &organization, nil
}

func assertEndpoint(t *testing.T, fhirClient fhirclient.Client, organizationURA string, connectionType string, connectionURLPath string) {
	org, err := searchOrg(fhirClient, organizationURA)
	require.NoError(t, err)
	require.NotNilf(t, org, "organization with URA %s should exist", organizationURA)
	for _, endpointRef := range org.Endpoint {
		var endpoint fhir.Endpoint
		err := fhirClient.Read(*endpointRef.Reference, &endpoint)
		require.NoError(t, err)
		if endpoint.ConnectionType.Code != nil && *endpoint.ConnectionType.Code == connectionType {
			assert.Truef(t, strings.HasSuffix(endpoint.Address, connectionURLPath), "endpoint address should end with %s", connectionURLPath)
			return
		}
	}
	t.Errorf("no endpoint with connection type %s found for organization with URA %s", connectionType, organizationURA)
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

func mapEntrySuffix(r mcsd.UpdateReport, suffix string) *mcsd.DirectoryUpdateReport {
	for key, value := range r {
		if strings.HasSuffix(key, suffix) {
			return &value
		}
	}
	return nil
}
