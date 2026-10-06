package mcsd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/from"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Test_mCSDUpdateClient(t *testing.T) {
	harnessDetail := harness.Start(t)
	t.Run("Force update mCSD Client", func(t *testing.T) {
		response := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		t.Run("assert resources synced from the central mCSD addressing directory", func(t *testing.T) {
			// The directory is registered directly (no discovery hop), so the first
			// sync creates every organization's Organization + Endpoint resources in
			// one pass: Sunflower (1 org + 2 endpoints), Plataan (1 org + 2 endpoints),
			// Care2Cure (1 org + 1 endpoint) = 8.
			assert.Equalf(t, 8, mapEntryContains(response, "lrza-root-directory").CountCreated, "created=8 in %v", response)
		})

		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
		t.Run("assert Sunflower organization resources", func(t *testing.T) {
			expectedOrg := lrza.CareHomeSunflower()
			org, err := searchOrg(queryFHIRClient, harnessDetail.SunflowerURA)
			require.NoError(t, err)
			assert.Equal(t, *expectedOrg.Name, *org.Name)
			assert.NotEqual(t, *expectedOrg.Id, *org.Id, "copy of organization in local Query Directory should have new ID")
			t.Run("meta", func(t *testing.T) {
				expectedSource := harnessDetail.LRZaFHIRBaseURL.JoinPath("Organization", *expectedOrg.Id)
				assert.Equal(t, expectedSource.String(), *org.Meta.Source, "copy of organization in local Query Directory should have Meta.Source set to original resource")
			})
			assertEndpoint(t, queryFHIRClient, harnessDetail.SunflowerURA, "hl7-fhir-rest", "fhir/sunflower-patients")
		})
		t.Run("assert Care2Cure organization resources", func(t *testing.T) {
			expectedOrg := lrza.Care2Cure()
			org, err := searchOrg(queryFHIRClient, lrza.Care2CureURA)
			require.NoError(t, err)
			assert.Equal(t, "Care2Cure Hospital", *org.Name)
			assert.NotEqual(t, *expectedOrg.Id, *org.Id, "copy of organization in local Query Directory should have new ID")
			t.Run("meta", func(t *testing.T) {
				expectedSource := harnessDetail.LRZaFHIRBaseURL.JoinPath("Organization", *expectedOrg.Id)
				assert.Equal(t, expectedSource.String(), *org.Meta.Source, "copy of organization in local Query Directory should have Meta.Source set to original resource")
			})
			assertEndpoint(t, queryFHIRClient, lrza.Care2CureURA, "hl7-fhir-rest", "/care2curehospital/fhir")
		})
	})
}

func Test_mCSDUpdateClient_IncrementalUpdates(t *testing.T) {
	t.Log("This test verifies that the mCSD update client correctly uses the _since parameter for incremental updates.")

	t.Run("updated endpoint in the central mCSD addressing directory (no references to other resources)", func(t *testing.T) {
		harnessDetail := harness.Start(t)
		t.Log("Initial sync")
		_ = invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)
		t.Log("Update Care2Cure's endpoint in the central addressing directory")
		newEndpoint := lrza.Care2CureEndpoints()[0]
		newEndpoint.Address = "https://example.com/updated/care2curehospital/fhir"
		lrzaFHIRClient := fhirclient.New(harnessDetail.LRZaFHIRBaseURL, http.DefaultClient, nil)
		err := lrzaFHIRClient.Update("Endpoint/"+*newEndpoint.Id, newEndpoint, nil)
		require.NoError(t, err, "Failed to update Care2Cure endpoint")

		t.Log("Second sync - should pick up updated endpoint via _since parameter")
		updateReport := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		lrzaReport := mapEntryContains(updateReport, "lrza-root-directory")
		require.Equal(t, 0, lrzaReport.CountCreated)
		// Organization history is always re-queried in full (no _since; see
		// queryAllResourceTypes), so all three organizations are unconditionally
		// resent as updates every sync, plus the one endpoint that actually changed.
		require.Equal(t, 4, lrzaReport.CountUpdated)

		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
		t.Run("assert updated endpoint in query directory", func(t *testing.T) {
			assertEndpoint(t, queryFHIRClient, lrza.Care2CureURA, "hl7-fhir-rest", "/updated/care2curehospital/fhir")
		})
	})
	t.Run("updated organization in the central mCSD addressing directory", func(t *testing.T) {
		t.Log("This test verifies that the mCSD update client resolves references to existing resources when updating a resource.")
		harnessDetail := harness.Start(t)
		t.Log("Initial sync")
		_ = invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)
		t.Log("Update Care2Cure's organization in the central addressing directory")
		updatedOrganization := lrza.Care2Cure()
		updatedOrganization.Alias = []string{"Updated Alias"}
		lrzaFHIRClient := fhirclient.New(harnessDetail.LRZaFHIRBaseURL, http.DefaultClient, nil)
		err := lrzaFHIRClient.Update("Organization/"+*updatedOrganization.Id, updatedOrganization, nil)
		require.NoError(t, err)

		updateReport := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		lrzaReport := mapEntryContains(updateReport, "lrza-root-directory")
		assert.Empty(t, lrzaReport.Warnings)
		assert.Empty(t, lrzaReport.Errors)
		assert.Equal(t, 0, lrzaReport.CountCreated)
		// All three organizations are unconditionally resent every sync (see above);
		// no endpoints changed, so only the organizations count as updates.
		assert.Equal(t, 3, lrzaReport.CountUpdated)

		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
		t.Run("assert updated organization in query directory", func(t *testing.T) {
			org, err := searchOrg(queryFHIRClient, lrza.Care2CureURA)
			require.NoError(t, err)
			require.NotNil(t, org)
			assert.Contains(t, org.Alias, "Updated Alias", "Organization alias should be updated")
		})
	})
	t.Run("new child organization in the central mCSD addressing directory", func(t *testing.T) {
		harnessDetail := harness.Start(t)
		// Test verifies _since parameter correctly enables incremental sync by:
		// 1. Doing baseline sync to establish timestamps
		// 2. Creating new child organization after sync completes
		// 3. Verifying next sync finds the new child organization via _since parameter
		// 4. Confirming subsequent sync finds nothing (no new changes)

		// First sync to establish baseline timestamps
		response1 := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		// First sync should behave like Test_mCSDUpdateClient - the directory should
		// create 8 resources (3 organizations + 5 endpoints).
		lrzaReport1 := mapEntryContains(response1, "lrza-root-directory")
		require.NotNil(t, lrzaReport1, "directory report should exist in first sync")
		assert.Equal(t, 8, lrzaReport1.CountCreated, "directory should create 8 resources in first sync")

		// Create new child organization after first sync - should be found by next incremental sync
		lrzaFHIRClient := fhirclient.New(harnessDetail.LRZaFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
			UsePostSearch: false,
		})

		// Find the parent organization with URA 00000030 in the central directory
		parentOrg, err := searchOrg(lrzaFHIRClient, lrza.Care2CureURA)
		require.NoError(t, err, "Failed to search for parent organization")
		require.NotNil(t, parentOrg, "Parent organization with URA 00000030 should exist")
		require.NotNil(t, parentOrg.Id, "Parent organization should have an ID")

		// Create child organization that is partOf the parent organization
		childOrgName := "Test Child Organization for Incremental Sync"
		identifierUseOfficial := fhir.IdentifierUseOfficial
		identifierSystem := "http://fhir.nl/fhir/NamingSystem/a_non_ura"
		childIdentifierValue := "incremental-test-999"
		partOfReference := "Organization/" + *parentOrg.Id

		childOrg := fhir.Organization{
			Name: &childOrgName,
			Identifier: []fhir.Identifier{
				{
					Use:    &identifierUseOfficial,
					System: &identifierSystem,
					Value:  &childIdentifierValue,
				},
			},
			PartOf: &fhir.Reference{
				Reference: &partOfReference,
			},
		}

		var createdChildOrg fhir.Organization
		err = lrzaFHIRClient.CreateWithContext(t.Context(), childOrg, &createdChildOrg)
		require.NoError(t, err, "Failed to create new child organization for incremental test")

		// Verify the child organization was actually created by reading it back
		var readBackChildOrg fhir.Organization
		err = lrzaFHIRClient.ReadWithContext(t.Context(), "Organization/"+*createdChildOrg.Id, &readBackChildOrg)
		require.NoError(t, err, "Failed to read back created child organization")
		require.Equal(t, childOrgName, *readBackChildOrg.Name, "Child organization name should match")
		require.NotNil(t, readBackChildOrg.PartOf, "Child organization should have partOf reference")

		// Second sync - should use _since and only find new resources (our test child organization)
		response2 := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		// Second sync should find our test child organization via _since parameter
		lrzaReport2 := mapEntryContains(response2, "lrza-root-directory")
		require.NotNil(t, lrzaReport2, "directory report should exist in second sync")
		assert.Equal(t, 1, lrzaReport2.CountCreated, "directory should find exactly 1 resource (our test child organization) via _since parameter")

		// Third sync - should find nothing (no new resources since second sync)
		response3 := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		// Third sync should find 0 resources (nothing new since second sync)
		lrzaReport3 := mapEntryContains(response3, "lrza-root-directory")
		require.NotNil(t, lrzaReport3, "directory report should exist in third sync")
		assert.Equal(t, 0, lrzaReport3.CountCreated, "directory should find 0 resources in third sync (nothing new)")
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

func mapEntryContains(r mcsd.UpdateReport, contained string) *mcsd.DirectoryUpdateReport {
	for key, value := range r {
		if strings.Contains(key, contained) {
			return &value
		}
	}
	return nil
}

func Test_DuplicateResourceHandling(t *testing.T) {
	// This test verifies that when _history returns multiple versions of the same resource,
	// the conditional _source updates work correctly and don't create duplicate resources

	harnessDetail := harness.Start(t)

	t.Run("POST+PUT+PUT scenario with child organization", func(t *testing.T) {
		// First, do an initial sync to handle any existing testdata
		_ = invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		lrzaFHIRClient := fhirclient.New(harnessDetail.LRZaFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
			UsePostSearch: false,
		})

		// Find the parent organization with URA 00000030 in the central directory
		parentOrg, err := searchOrg(lrzaFHIRClient, lrza.Care2CureURA)
		require.NoError(t, err, "Failed to search for parent organization")
		require.NotNil(t, parentOrg, "Parent organization with URA 00000030 should exist")
		require.NotNil(t, parentOrg.Id, "Parent organization should have an ID")

		// 1. Create child organization (POST) that is partOf the parent organization
		childOrgName := "Test Child Organization"
		identifierUseOfficial := fhir.IdentifierUseOfficial
		identifierSystem := "http://fhir.nl/fhir/NamingSystem/a_non_ura"
		childIdentifierValue := "child-test-456"
		active := true
		partOfReference := "Organization/" + *parentOrg.Id

		childOrg := fhir.Organization{
			Name:   &childOrgName,
			Active: &active,
			Identifier: []fhir.Identifier{
				{
					Use:    &identifierUseOfficial,
					System: &identifierSystem,
					Value:  &childIdentifierValue,
				},
			},
			PartOf: &fhir.Reference{
				Reference: &partOfReference,
			},
		}

		var createdChildOrg fhir.Organization
		err = lrzaFHIRClient.CreateWithContext(t.Context(), childOrg, &createdChildOrg)
		require.NoError(t, err, "Failed to create child organization")

		// 2. Update child organization (first PUT)
		updatedName1 := "Test Child Organization - Updated 1"
		createdChildOrg.Name = &updatedName1

		var updatedChildOrg1 fhir.Organization
		err = lrzaFHIRClient.UpdateWithContext(t.Context(), "Organization/"+*createdChildOrg.Id, createdChildOrg, &updatedChildOrg1)
		require.NoError(t, err, "Failed to update child organization (first time)")

		// 3. Update child organization again (second PUT)
		updatedName2 := "Test Child Organization - Updated 2"
		updatedChildOrg1.Name = &updatedName2

		var updatedChildOrg2 fhir.Organization
		err = lrzaFHIRClient.UpdateWithContext(t.Context(), "Organization/"+*updatedChildOrg1.Id, updatedChildOrg1, &updatedChildOrg2)
		require.NoError(t, err, "Failed to update child organization (second time)")

		// Verify the source child organization now has version 3 after POST(v1) + PUT(v2) + PUT(v3)
		require.NotNil(t, updatedChildOrg2.Meta, "Updated child organization should have meta")
		require.NotNil(t, updatedChildOrg2.Meta.VersionId, "Updated child organization should have version ID")
		assert.Equal(t, "3", *updatedChildOrg2.Meta.VersionId, "Source server should assign version 3 after POST+PUT+PUT sequence")

		// 4. Now run mCSD sync to see how it handles the POST+PUT+PUT history for child organization
		updateReport := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		// Check that no errors occurred during sync
		lrzaReport := mapEntryContains(updateReport, "lrza-root-directory")
		require.NotNil(t, lrzaReport, "directory report should exist")
		require.Empty(t, lrzaReport.Errors, "Should not have errors with conditional _source updates")

		// 5. Verify only ONE child organization exists in query directory with the latest name
		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)

		// Search for child organizations with our test identifier
		searchResults := fhir.Bundle{}
		err = queryFHIRClient.SearchWithContext(t.Context(), "Organization", url.Values{
			"identifier": []string{identifierSystem + "|" + childIdentifierValue},
		}, &searchResults)
		require.NoError(t, err, "Failed to search for child organizations in query directory")

		// Should find exactly ONE child organization (not duplicates) after deduplication
		require.Len(t, searchResults.Entry, 1, "Should have exactly 1 child organization in query directory after POST+PUT+PUT deduplication")

		// Verify it has the latest name (from the second update)
		var foundChildOrg fhir.Organization
		require.NoError(t, json.Unmarshal(searchResults.Entry[0].Resource, &foundChildOrg))
		assert.Equal(t, "Test Child Organization - Updated 2", *foundChildOrg.Name, "Should have the latest version of the child organization")

		// Verify the partOf reference is preserved
		require.NotNil(t, foundChildOrg.PartOf, "Child organization should have partOf reference")
		require.NotNil(t, foundChildOrg.PartOf.Reference, "Child organization partOf should have reference")
		assert.Contains(t, *foundChildOrg.PartOf.Reference, "Organization/", "Child organization partOf should reference parent organization")

		// Verify it has the expected version ID
		// Source server: POST(v1) + PUT(v2) + PUT(v3) = version 3
		// Query server: receives deduped resource and creates it as version 1
		require.NotNil(t, foundChildOrg.Meta, "Child organization should have meta")
		require.NotNil(t, foundChildOrg.Meta.VersionId, "Child organization should have version ID")
		assert.Equal(t, "1", *foundChildOrg.Meta.VersionId, "Query server should assign version 1 to the synchronized resource")
	})

	t.Run("CREATE+DELETE scenario with child organization", func(t *testing.T) {
		// First, do an initial sync to handle any existing testdata
		_ = invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		lrzaFHIRClient := fhirclient.New(harnessDetail.LRZaFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
			UsePostSearch: false,
		})

		// Find the parent organization with URA 00000030 in the central directory
		parentOrg, err := searchOrg(lrzaFHIRClient, lrza.Care2CureURA)
		require.NoError(t, err, "Failed to search for parent organization")
		require.NotNil(t, parentOrg, "Parent organization with URA 00000030 should exist")
		require.NotNil(t, parentOrg.Id, "Parent organization should have an ID")

		// 1. Create child organization (POST) that is partOf the parent organization
		childOrgName := "Test Child Organization for Deletion"
		identifierUseOfficial := fhir.IdentifierUseOfficial
		identifierSystem := "http://fhir.nl/fhir/NamingSystem/a_non_ura"
		childIdentifierValue := "delete-test-789"
		active := true
		partOfReference := "Organization/" + *parentOrg.Id

		childOrg := fhir.Organization{
			Name:   &childOrgName,
			Active: &active,
			Identifier: []fhir.Identifier{
				{
					Use:    &identifierUseOfficial,
					System: &identifierSystem,
					Value:  &childIdentifierValue,
				},
			},
			PartOf: &fhir.Reference{
				Reference: &partOfReference,
			},
		}

		var createdChildOrg fhir.Organization
		err = lrzaFHIRClient.CreateWithContext(t.Context(), childOrg, &createdChildOrg)
		require.NoError(t, err, "Failed to create child organization for deletion test")

		// 2. First sync - should create the child organization in query directory
		_ = invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		// Verify child organization exists in query directory
		queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
		searchResults1 := fhir.Bundle{}
		err = queryFHIRClient.SearchWithContext(t.Context(), "Organization", url.Values{
			"identifier": []string{identifierSystem + "|" + childIdentifierValue},
		}, &searchResults1)
		require.NoError(t, err, "Failed to search for child organizations in query directory")
		require.Len(t, searchResults1.Entry, 1, "Should have 1 child organization in query directory before deletion")

		// Verify the partOf reference exists
		var foundChildOrg1 fhir.Organization
		require.NoError(t, json.Unmarshal(searchResults1.Entry[0].Resource, &foundChildOrg1))
		require.NotNil(t, foundChildOrg1.PartOf, "Child organization should have partOf reference before deletion")

		// 3. Delete the child organization from source
		err = lrzaFHIRClient.DeleteWithContext(t.Context(), "Organization/"+*createdChildOrg.Id)
		require.NoError(t, err, "Failed to delete child organization from source")

		// 4. Second sync - should process the deletion
		updateReport := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

		lrzaReport2 := mapEntryContains(updateReport, "lrza-root-directory")
		require.NotNil(t, lrzaReport2, "directory report should exist after deletion")

		// 5. Verify child organization is deleted from query directory
		searchResults2 := fhir.Bundle{}
		err = queryFHIRClient.SearchWithContext(t.Context(), "Organization", url.Values{
			"identifier": []string{identifierSystem + "|" + childIdentifierValue},
		}, &searchResults2)
		require.NoError(t, err, "Failed to search for child organizations in query directory after deletion")

		// DELETE operations are now properly processed using conditional _source deletions
		// The child organization should be removed from the query directory
		require.Len(t, searchResults2.Entry, 0, "Should have 0 child organizations in query directory after DELETE is processed")

		// Verify the DeleteCount is 1 in the sync report (confirming DELETE was processed)
		require.Equal(t, 1, lrzaReport2.CountDeleted, "DELETE operations should be processed and counted")
	})
}

func Test_URAIdentifierChange(t *testing.T) {
	t.Log("This test verifies that when an Organization's URA identifier changes, the system reruns the history query without _since parameter")

	harnessDetail := harness.Start(t)

	lrzaFHIRClient := fhirclient.New(harnessDetail.LRZaFHIRBaseURL, http.DefaultClient, &fhirclient.Config{
		UsePostSearch: false,
	})

	// Find the parent organization with URA 00000030
	authoritativeURA := lrza.Care2CureURA
	nonAuthoritativeURA := "99999999"

	parentOrg, err := searchOrg(lrzaFHIRClient, authoritativeURA)
	require.NoError(t, err, "Failed to search for parent organization")
	require.NotNil(t, parentOrg, "Parent organization with URA 00000030 should exist")
	require.NotNil(t, parentOrg.Id, "Parent organization should have an ID")

	t.Log("Step 1: Change organization URA to non-authoritative URA (99999999)")
	// Change the URA identifier to a non-authoritative value
	updatedOrg := *parentOrg
	for i, identifier := range updatedOrg.Identifier {
		if identifier.System != nil && *identifier.System == coding.URANamingSystem {
			updatedOrg.Identifier[i].Value = &nonAuthoritativeURA
			break
		}
	}

	err = lrzaFHIRClient.Update("Organization/"+*updatedOrg.Id, updatedOrg, nil)
	require.NoError(t, err, "Failed to update organization URA identifier")

	// Verify the URA was changed to non-authoritative value
	var verifyOrg fhir.Organization
	err = lrzaFHIRClient.Read("Organization/"+*updatedOrg.Id, &verifyOrg)
	require.NoError(t, err)
	foundNonAuthURA := false
	for _, identifier := range verifyOrg.Identifier {
		if identifier.System != nil && *identifier.System == coding.URANamingSystem {
			require.Equal(t, nonAuthoritativeURA, *identifier.Value, "URA should be updated to non-authoritative value")
			foundNonAuthURA = true
			break
		}
	}
	require.True(t, foundNonAuthURA, "Non-authoritative URA should be present in updated organization")

	t.Log("Step 2: First sync - non-authoritative URA")
	response1 := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)
	require.NotNil(t, response1)

	lrzaReport1 := mapEntryContains(response1, "lrza-root-directory")
	require.NotNil(t, lrzaReport1, "directory report should exist in first sync")

	// Log the first sync report
	t.Logf("First sync report: Created=%d, Updated=%d, Deleted=%d",
		lrzaReport1.CountCreated, lrzaReport1.CountUpdated, lrzaReport1.CountDeleted)

	// The organization with non-authoritative URA won't be synced to query directory
	firstSyncTotal := lrzaReport1.CountCreated + lrzaReport1.CountUpdated

	t.Log("Step 3: Change organization URA back to authoritative URA (00000030)")
	// Now change it back to the authoritative URA
	updatedOrg2 := verifyOrg
	for i, identifier := range updatedOrg2.Identifier {
		if identifier.System != nil && *identifier.System == coding.URANamingSystem {
			updatedOrg2.Identifier[i].Value = &authoritativeURA
			break
		}
	}

	err = lrzaFHIRClient.Update("Organization/"+*updatedOrg2.Id, updatedOrg2, nil)
	require.NoError(t, err, "Failed to update organization URA back to authoritative value")

	// Verify the URA was changed back
	var verifyOrg2 fhir.Organization
	err = lrzaFHIRClient.Read("Organization/"+*updatedOrg2.Id, &verifyOrg2)
	require.NoError(t, err)
	foundAuthURA := false
	for _, identifier := range verifyOrg2.Identifier {
		if identifier.System != nil && *identifier.System == coding.URANamingSystem {
			require.Equal(t, authoritativeURA, *identifier.Value, "URA should be back to authoritative value")
			foundAuthURA = true
			break
		}
	}
	require.True(t, foundAuthURA, "Authoritative URA should be present in updated organization")

	t.Log("Step 4: Second sync - should detect URA change and rerun without _since, syncing all resources")
	response2 := invokeUpdate(t, harnessDetail.KnooppuntInternalBaseURL)

	lrzaReport2 := mapEntryContains(response2, "lrza-root-directory")
	require.NotNil(t, lrzaReport2, "directory report should exist in second sync")

	// Log the second sync report
	t.Logf("Second sync report: Created=%d, Updated=%d, Deleted=%d, Errors=%v, Warnings=%v",
		lrzaReport2.CountCreated, lrzaReport2.CountUpdated, lrzaReport2.CountDeleted,
		lrzaReport2.Errors, lrzaReport2.Warnings)

	// The system should have detected the URA change (from non-auth to auth) and rerun the full query
	// This should sync MORE resources than the first sync because now the authoritative URA is present
	secondSyncTotal := lrzaReport2.CountCreated + lrzaReport2.CountUpdated
	t.Logf("First sync total: %d, Second sync total: %d", firstSyncTotal, secondSyncTotal)

	require.True(t, secondSyncTotal > firstSyncTotal,
		"Second sync should process more resources after URA changed from non-authoritative to authoritative")

	// Verify no errors occurred during the sync
	assert.Empty(t, lrzaReport2.Errors, "Should have no errors during sync with URA change")

	// Verify the organization is searchable by the authoritative URA in the query directory
	queryFHIRClient := fhirclient.New(harnessDetail.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)

	t.Log("Step 5: Verifying organization with authoritative URA in query directory")

	t.Run("assert organization with authoritative URA exists in query directory", func(t *testing.T) {
		org, err := searchOrg(queryFHIRClient, authoritativeURA)
		require.NoError(t, err)
		require.NotNil(t, org, "Organization with authoritative URA should exist in query directory")
		assert.Equal(t, *parentOrg.Name, *org.Name, "Organization name should match")
	})

	t.Run("non-authoritative URA should not return the organization", func(t *testing.T) {
		// The non-authoritative URA was replaced, so searching by it should return nothing
		org, err := searchOrg(queryFHIRClient, nonAuthoritativeURA)
		require.NoError(t, err)
		require.Nil(t, org, "Organization with non-authoritative URA should not exist in query directory")
	})
}

func invokeUpdate(t *testing.T, baseURL *url.URL) mcsd.UpdateReport {
	httpResponse, err := http.Post(baseURL.JoinPath("mcsd/update").String(), "application/json", nil)
	require.NoError(t, err)
	updateReport, err := from.JSONResponse[mcsd.UpdateReport](httpResponse)
	require.NoError(t, err)
	return updateReport
}
