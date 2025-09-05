package mcsd

import (
	"context"
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

func Test_mCSDUpdateClient_IncrementalUpdates(t *testing.T) {
	harnessDetail := harness.Start(t)

	t.Run("Test incremental updates with _since parameter", func(t *testing.T) {
		// Test verifies _since parameter correctly enables incremental sync by:
		// 1. Doing baseline sync to establish timestamps
		// 2. Creating new organization after sync completes  
		// 3. Verifying next sync finds the new organization via _since parameter
		// 4. Confirming subsequent sync finds nothing (no new changes)

		// First sync to establish baseline timestamps
		httpResponse1, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse1.StatusCode)
		responseData1, err := io.ReadAll(httpResponse1.Body)
		require.NoError(t, err)

		var response1 mcsd.UpdateReport
		require.NoError(t, json.Unmarshal(responseData1, &response1))

		// First sync should behave like Test_mCSDUpdateClient - LRZa should create 2 resources
		lrzaReport1 := mapEntrySuffix(response1, "lrza-mcsd-admin")
		require.NotNil(t, lrzaReport1, "LRZa report should exist in first sync")
		assert.Equal(t, 2, lrzaReport1.CountCreated, "LRZa should create 2 resources in first sync")

		// Create new organization after first sync - should be found by next incremental sync  
		// Use discovered directory (care2cure-admin) since they sync all resource types including Organizations
		care2CureFHIRClient := fhirclient.New(harnessDetail.Care2CureFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
			UsePostSearch: false,
		})
		orgName := "Test Organization for Incremental Sync"
		identifierUseOfficial := fhir.IdentifierUseOfficial
		identifierSystem := "http://fhir.nl/fhir/NamingSystem/ura"
		identifierValue := "99999999"
		newOrg := fhir.Organization{
			Name: &orgName,
			Identifier: []fhir.Identifier{
				{
					Use:    &identifierUseOfficial,
					System: &identifierSystem,
					Value:  &identifierValue,
				},
			},
		}

		var createdOrg fhir.Organization
		err = care2CureFHIRClient.CreateWithContext(context.Background(), newOrg, &createdOrg)
		require.NoError(t, err, "Failed to create new organization for incremental test")

		// Verify the organization was actually created by reading it back
		var readBackOrg fhir.Organization
		err = care2CureFHIRClient.ReadWithContext(context.Background(), "Organization/"+*createdOrg.Id, &readBackOrg)
		require.NoError(t, err, "Failed to read back created organization")
		require.Equal(t, orgName, *readBackOrg.Name, "Organization name should match")

		// Second sync - should use _since and only find new resources (our test organization)
		httpResponse2, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse2.StatusCode)
		responseData2, err := io.ReadAll(httpResponse2.Body)
		require.NoError(t, err)

		var response2 mcsd.UpdateReport
		require.NoError(t, json.Unmarshal(responseData2, &response2))

		// Second sync should find our test organization via _since parameter  
		care2CureReport2 := mapEntrySuffix(response2, "care2cure-admin")
		require.NotNil(t, care2CureReport2, "Care2Cure report should exist in second sync")
		assert.Equal(t, 1, care2CureReport2.CountCreated, "Care2Cure should find exactly 1 resource (our test organization) via _since parameter")

		// Third sync - should find nothing (no new resources since second sync)
		httpResponse3, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse3.StatusCode)
		responseData3, err := io.ReadAll(httpResponse3.Body)
		require.NoError(t, err)

		var response3 mcsd.UpdateReport
		require.NoError(t, json.Unmarshal(responseData3, &response3))

		// Third sync should find 0 resources (nothing new since second sync)
		care2CureReport3 := mapEntrySuffix(response3, "care2cure-admin")
		require.NotNil(t, care2CureReport3, "Care2Cure report should exist in third sync")
		assert.Equal(t, 0, care2CureReport3.CountCreated, "Care2Cure should find 0 resources in third sync (nothing new)")
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

func mapEntrySuffix(r mcsd.UpdateReport, suffix string) *mcsd.DirectoryUpdateReport {
	for key, value := range r {
		if strings.HasSuffix(key, suffix) {
			return &value
		}
	}
	return nil
}
