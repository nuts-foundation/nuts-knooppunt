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

func Test_DuplicateResourceHandling(t *testing.T) {
	// This test verifies that when _history returns multiple versions of the same resource,
	// the conditional _source updates work correctly and don't create duplicate resources

	harnessDetail := harness.Start(t)

	t.Run("POST+PUT+PUT scenario with same resource", func(t *testing.T) {
		// Use care2cure FHIR server as the source (discovered directory)
		care2CureFHIRClient := fhirclient.New(harnessDetail.Care2CureFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
			UsePostSearch: false,
		})

		// 1. Create standalone organization (POST) - no references to avoid UUID resolution issues
		orgName := "Test Duplicate Organization"
		identifierUseOfficial := fhir.IdentifierUseOfficial
		identifierSystem := "http://fhir.nl/fhir/NamingSystem/ura"
		identifierValue := "duplicate-test-123"
		active := true

		newOrg := fhir.Organization{
			Name:   &orgName,
			Active: &active,
			Identifier: []fhir.Identifier{
				{
					Use:    &identifierUseOfficial,
					System: &identifierSystem,
					Value:  &identifierValue,
				},
			},
			// Don't add endpoint references to avoid UUID resolution issues
		}

		var createdOrg fhir.Organization
		err := care2CureFHIRClient.CreateWithContext(context.Background(), newOrg, &createdOrg)
		require.NoError(t, err, "Failed to create organization")

		// 2. Update organization (first PUT)
		updatedName1 := "Test Duplicate Organization - Updated 1"
		createdOrg.Name = &updatedName1

		var updatedOrg1 fhir.Organization
		err = care2CureFHIRClient.UpdateWithContext(context.Background(), "Organization/"+*createdOrg.Id, createdOrg, &updatedOrg1)
		require.NoError(t, err, "Failed to update organization (first time)")

		// 3. Update organization again (second PUT)
		updatedName2 := "Test Duplicate Organization - Updated 2"
		updatedOrg1.Name = &updatedName2

		var updatedOrg2 fhir.Organization
		err = care2CureFHIRClient.UpdateWithContext(context.Background(), "Organization/"+*updatedOrg1.Id, updatedOrg1, &updatedOrg2)
		require.NoError(t, err, "Failed to update organization (second time)")
		
		// Verify the source organization now has version 3 after POST(v1) + PUT(v2) + PUT(v3)
		require.NotNil(t, updatedOrg2.Meta, "Updated organization should have meta")
		require.NotNil(t, updatedOrg2.Meta.VersionId, "Updated organization should have version ID")
		assert.Equal(t, "3", *updatedOrg2.Meta.VersionId, "Source server should assign version 3 after POST+PUT+PUT sequence")

		// 4. Now run mCSD sync to see how it handles the POST+PUT+PUT history
		httpResponse, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse.StatusCode)

		responseData, err := io.ReadAll(httpResponse.Body)
		require.NoError(t, err)
		t.Logf("mCSD sync response: %s", string(responseData))

		var updateReport mcsd.UpdateReport
		require.NoError(t, json.Unmarshal(responseData, &updateReport))

		// Check that no errors occurred during sync
		care2CureReport := mapEntrySuffix(updateReport, "care2cure-admin")
		require.NotNil(t, care2CureReport, "Care2Cure report should exist")
		require.Empty(t, care2CureReport.Errors, "Should not have errors with conditional _source updates")

		// 5. Verify only ONE organization exists in query directory with the latest name
		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)

		// Search for organizations with our test identifier
		searchResults := fhir.Bundle{}
		err = queryFHIRClient.SearchWithContext(context.Background(), "Organization", url.Values{
			"identifier": []string{identifierSystem + "|" + identifierValue},
		}, &searchResults)
		require.NoError(t, err, "Failed to search for organizations in query directory")

		// Should find exactly ONE organization (not duplicates) after deduplication
		require.Len(t, searchResults.Entry, 1, "Should have exactly 1 organization in query directory after POST+PUT+PUT deduplication")

		// Verify it has the latest name (from the second update)
		var foundOrg fhir.Organization
		require.NoError(t, json.Unmarshal(searchResults.Entry[0].Resource, &foundOrg))
		assert.Equal(t, "Test Duplicate Organization - Updated 2", *foundOrg.Name, "Should have the latest version of the organization")

		// Verify it has the expected version ID
		// Source server: POST(v1) + PUT(v2) + PUT(v3) = version 3
		// Query server: receives deduped resource and creates it as version 1
		require.NotNil(t, foundOrg.Meta, "Organization should have meta")
		require.NotNil(t, foundOrg.Meta.VersionId, "Organization should have version ID")
		assert.Equal(t, "1", *foundOrg.Meta.VersionId, "Query server should assign version 1 to the synchronized resource")
		t.Logf("Found organization with expected version: %s", *foundOrg.Meta.VersionId)

		t.Logf("Successfully handled POST+PUT+PUT scenario - found 1 organization with latest name: %s", *foundOrg.Name)
	})

	t.Run("CREATE+DELETE scenario", func(t *testing.T) {
		// Use care2cure FHIR server as the source (discovered directory)
		care2CureFHIRClient := fhirclient.New(harnessDetail.Care2CureFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
			UsePostSearch: false,
		})

		// 1. Create organization (POST)
		orgName := "Test Organization for Deletion"
		identifierUseOfficial := fhir.IdentifierUseOfficial
		identifierSystem := "http://fhir.nl/fhir/NamingSystem/ura"
		identifierValue := "delete-test-456"
		active := true

		newOrg := fhir.Organization{
			Name:   &orgName,
			Active: &active,
			Identifier: []fhir.Identifier{
				{
					Use:    &identifierUseOfficial,
					System: &identifierSystem,
					Value:  &identifierValue,
				},
			},
		}

		var createdOrg fhir.Organization
		err := care2CureFHIRClient.CreateWithContext(context.Background(), newOrg, &createdOrg)
		require.NoError(t, err, "Failed to create organization for deletion test")

		// 2. First sync - should create the organization in query directory
		httpResponse1, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse1.StatusCode)

		// Verify organization exists in query directory
		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
		searchResults1 := fhir.Bundle{}
		err = queryFHIRClient.SearchWithContext(context.Background(), "Organization", url.Values{
			"identifier": []string{identifierSystem + "|" + identifierValue},
		}, &searchResults1)
		require.NoError(t, err, "Failed to search for organizations in query directory")
		require.Len(t, searchResults1.Entry, 1, "Should have 1 organization in query directory before deletion")

		// 3. Delete the organization from source
		err = care2CureFHIRClient.DeleteWithContext(context.Background(), "Organization/"+*createdOrg.Id)
		require.NoError(t, err, "Failed to delete organization from source")

		// 4. Second sync - should process the deletion
		httpResponse2, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse2.StatusCode)

		responseData2, err := io.ReadAll(httpResponse2.Body)
		require.NoError(t, err)
		t.Logf("mCSD sync response after deletion: %s", string(responseData2))

		var response2 mcsd.UpdateReport
		require.NoError(t, json.Unmarshal(responseData2, &response2))

		care2CureReport2 := mapEntrySuffix(response2, "care2cure-admin")
		require.NotNil(t, care2CureReport2, "Care2Cure report should exist after deletion")

		// 5. Verify organization is deleted from query directory
		searchResults2 := fhir.Bundle{}
		err = queryFHIRClient.SearchWithContext(context.Background(), "Organization", url.Values{
			"identifier": []string{identifierSystem + "|" + identifierValue},
		}, &searchResults2)
		require.NoError(t, err, "Failed to search for organizations in query directory after deletion")

		// On main branch, DELETE operations are skipped for safety (until _source conditional updates are implemented)
		// So the organization should still exist in the query directory
		require.Len(t, searchResults2.Entry, 1, "Should still have 1 organization in query directory (DELETE operations are skipped on main branch)")

		// Verify the DeleteCount is 0 in the sync report (confirming DELETE was skipped)
		require.Equal(t, 0, care2CureReport2.CountDeleted, "DELETE operations should be skipped on main branch")

		t.Logf("Successfully handled CREATE+DELETE scenario - DELETE operation was skipped as expected on main branch")
	})
}
