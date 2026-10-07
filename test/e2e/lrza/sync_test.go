package lrza

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// locationCount is the number of extra Locations put in the Central Addressing
// Directory before the initial load. It exceeds the update client's page size
// (100), so the initial load only completes if it follows Bundle.link[next].
const locationCount = 150

// testIdentifierSystem marks the resources this test adds to the Central
// Addressing Directory, so their copies can be found in the Local Query
// Directory.
const testIdentifierSystem = "http://example.org/lrza-e2e"

// Test_LRZASync drives the LRZA update client through
// one initial load and a series of incremental syncs against a single harness.
// Every step changes the Central Addressing Directory, syncs, and asserts the
// report and the resulting Local Query Directory. The steps build on each
// other's state, so the test stops at the first failing step.
func Test_LRZASync(t *testing.T) {
	h := harness.Start(t)
	source := fhirclient.New(h.LRZaFHIRBaseURL, http.DefaultClient, nil)
	query := fhirclient.New(h.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)

	care2Cure := lrza.Care2Cure()
	care2CureEndpoint := lrza.Care2CureEndpoints()[0]
	ward := lrza.CareHomeSunflowerHealthcareServices()[0]

	// initialAffiliation is synced in the initial load. It is the last resource
	// type synced and references all the others, including the seeded
	// HealthcareService and Locations from a separate (paginated) search.
	initialAffiliation := fhir.OrganizationAffiliation{
		Id:                        new("lrza-e2e-initial-affiliation"),
		Identifier:                []fhir.Identifier{{System: new(testIdentifierSystem), Value: new("initial-affiliation")}},
		Organization:              &fhir.Reference{Reference: new("Organization/" + *lrza.HospitalPlataan().Id)},
		ParticipatingOrganization: &fhir.Reference{Reference: new("Organization/" + *lrza.CareHomeSunflower().Id)},
		Location: []fhir.Reference{
			{Reference: new("Location/" + locationID(0))},
			{Reference: new("Location/" + locationID(locationCount-1))},
		},
		HealthcareService: []fhir.Reference{{Reference: new("HealthcareService/" + *ward.Id)}},
		Endpoint:          []fhir.Reference{{Reference: ward.Endpoint[0].Reference}},
	}

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"initial load follows pagination", func(t *testing.T) {
			putLocations(t, source, *care2Cure.Id)
			require.NoError(t, source.Update("OrganizationAffiliation/"+*initialAffiliation.Id, initialAffiliation, nil))

			report := sync(t, h.KnooppuntInternalBaseURL)

			// The vectors, plus the Locations and the affiliation added above.
			assertReport(t, report, len(lrza.Resources())+locationCount+1, 0)
			assert.Equal(t, locationCount, countByIdentifierSystem(t, query, "Location"))
		}},
		{"initial load copies organizations with their endpoints", func(t *testing.T) {
			for _, expected := range []struct {
				organization fhir.Organization
				ura          string
				endpoints    []fhir.Endpoint
			}{
				{lrza.CareHomeSunflower(), sunflower.URA, lrza.CareHomeSunflowerEndpoints()},
				{lrza.HospitalPlataan(), plataan.URA, lrza.HospitalPlataanEndpoints()},
				{lrza.Care2Cure(), lrza.Care2CureURA, lrza.Care2CureEndpoints()},
			} {
				t.Run(*expected.organization.Name, func(t *testing.T) {
					org := searchOrg(t, query, expected.ura)
					assert.Equal(t, *expected.organization.Name, *org.Name)
					assert.NotEqual(t, *expected.organization.Id, *org.Id, "the local copy gets its own id")
					assert.Equal(t, sourceURL(h, "Organization", *expected.organization.Id), *org.Meta.Source)

					// Organization.endpoint references are rewritten to the local
					// copies of the source's Endpoints.
					var gotAddresses []string
					for _, ref := range org.Endpoint {
						var endpoint fhir.Endpoint
						require.NoError(t, query.Read(*ref.Reference, &endpoint), "reference %s should resolve locally", *ref.Reference)
						gotAddresses = append(gotAddresses, endpoint.Address)
					}
					var expectedAddresses []string
					for _, endpoint := range expected.endpoints {
						expectedAddresses = append(expectedAddresses, endpoint.Address)
					}
					assert.ElementsMatch(t, expectedAddresses, gotAddresses)
				})
			}
		}},
		{"initial load resolves references across resource types", func(t *testing.T) {
			localSunflower := searchOrg(t, query, sunflower.URA)
			localPlataan := searchOrg(t, query, plataan.URA)
			localWardEndpoint := readLocalCopy[fhir.Endpoint](t, h, query, "Endpoint", *lrza.CareHomeSunflowerEndpoints()[0].Id)
			localWard := readLocalCopy[fhir.HealthcareService](t, h, query, "HealthcareService", *ward.Id)

			// HealthcareService is synced before Endpoint, so its endpoint
			// reference points forward in the sync order.
			assert.Equal(t, *ward.Name, *localWard.Name)
			assert.Equal(t, "Organization/"+*localSunflower.Id, *localWard.ProvidedBy.Reference)
			require.Len(t, localWard.Endpoint, 1)
			assert.Equal(t, "Endpoint/"+*localWardEndpoint.Id, *localWard.Endpoint[0].Reference)

			localAffiliation := readLocalCopy[fhir.OrganizationAffiliation](t, h, query, "OrganizationAffiliation", *initialAffiliation.Id)
			assert.Equal(t, "Organization/"+*localPlataan.Id, *localAffiliation.Organization.Reference)
			assert.Equal(t, "Organization/"+*localSunflower.Id, *localAffiliation.ParticipatingOrganization.Reference)
			require.Len(t, localAffiliation.HealthcareService, 1)
			assert.Equal(t, "HealthcareService/"+*localWard.Id, *localAffiliation.HealthcareService[0].Reference)
			require.Len(t, localAffiliation.Endpoint, 1)
			assert.Equal(t, "Endpoint/"+*localWardEndpoint.Id, *localAffiliation.Endpoint[0].Reference)
			require.Len(t, localAffiliation.Location, 2)
			for i, sourceID := range []string{locationID(0), locationID(locationCount - 1)} {
				localLocation := readLocalCopy[fhir.Location](t, h, query, "Location", sourceID)
				assert.Equal(t, "Location/"+*localLocation.Id, *localAffiliation.Location[i].Reference)
			}
		}},
		{"re-sync without changes is a no-op", func(t *testing.T) {
			report := sync(t, h.KnooppuntInternalBaseURL)

			assertReport(t, report, 0, 0)
			assert.Equal(t, locationCount, countByIdentifierSystem(t, query, "Location"))
		}},
		{"incremental sync applies an update", func(t *testing.T) {
			care2CureEndpoint.Address = "https://example.com/updated/care2curehospital/fhir"
			require.NoError(t, source.Update("Endpoint/"+*care2CureEndpoint.Id, care2CureEndpoint, nil))

			report := sync(t, h.KnooppuntInternalBaseURL)

			assertReport(t, report, 0, 1)
			endpoint := readLocalCopy[fhir.Endpoint](t, h, query, "Endpoint", *care2CureEndpoint.Id)
			assert.Equal(t, care2CureEndpoint.Address, endpoint.Address)
		}},
		{"incremental sync creates resources referencing synced ones", func(t *testing.T) {
			healthcareService := fhir.HealthcareService{
				Id:         new("lrza-e2e-healthcareservice"),
				Identifier: []fhir.Identifier{{System: new(testIdentifierSystem), Value: new("healthcareservice")}},
				Name:       new("Care2Cure Cardiology"),
				ProvidedBy: &fhir.Reference{Reference: new("Organization/" + *care2Cure.Id)},
				Endpoint:   []fhir.Reference{{Reference: new("Endpoint/" + *care2CureEndpoint.Id)}},
			}
			affiliation := fhir.OrganizationAffiliation{
				Id:                        new("lrza-e2e-affiliation"),
				Identifier:                []fhir.Identifier{{System: new(testIdentifierSystem), Value: new("affiliation")}},
				Organization:              &fhir.Reference{Reference: new("Organization/" + *lrza.HospitalPlataan().Id)},
				ParticipatingOrganization: &fhir.Reference{Reference: new("Organization/" + *lrza.CareHomeSunflower().Id)},
			}
			require.NoError(t, source.Update("HealthcareService/"+*healthcareService.Id, healthcareService, nil))
			require.NoError(t, source.Update("OrganizationAffiliation/"+*affiliation.Id, affiliation, nil))

			report := sync(t, h.KnooppuntInternalBaseURL)

			assertReport(t, report, 2, 0)
			localCare2Cure := searchOrg(t, query, lrza.Care2CureURA)
			localEndpoint := readLocalCopy[fhir.Endpoint](t, h, query, "Endpoint", *care2CureEndpoint.Id)
			localService := readLocalCopy[fhir.HealthcareService](t, h, query, "HealthcareService", *healthcareService.Id)
			assert.Equal(t, "Organization/"+*localCare2Cure.Id, *localService.ProvidedBy.Reference)
			require.Len(t, localService.Endpoint, 1)
			assert.Equal(t, "Endpoint/"+*localEndpoint.Id, *localService.Endpoint[0].Reference)

			localPlataan := searchOrg(t, query, plataan.URA)
			localSunflower := searchOrg(t, query, sunflower.URA)
			localAffiliation := readLocalCopy[fhir.OrganizationAffiliation](t, h, query, "OrganizationAffiliation", *affiliation.Id)
			assert.Equal(t, "Organization/"+*localPlataan.Id, *localAffiliation.Organization.Reference)
			assert.Equal(t, "Organization/"+*localSunflower.Id, *localAffiliation.ParticipatingOrganization.Reference)
		}},
		{"incremental sync applies a withdrawal through status", func(t *testing.T) {
			care2CureEndpoint.Status = fhir.EndpointStatusOff
			require.NoError(t, source.Update("Endpoint/"+*care2CureEndpoint.Id, care2CureEndpoint, nil))

			report := sync(t, h.KnooppuntInternalBaseURL)

			assertReport(t, report, 0, 1)
			endpoint := readLocalCopy[fhir.Endpoint](t, h, query, "Endpoint", *care2CureEndpoint.Id)
			assert.Equal(t, fhir.EndpointStatusOff, endpoint.Status)
		}},
		{"incremental sync applies only the newest of several versions", func(t *testing.T) {
			var healthcareService fhir.HealthcareService
			require.NoError(t, source.Read("HealthcareService/lrza-e2e-healthcareservice", &healthcareService))
			for _, name := range []string{"Care2Cure Cardiology v2", "Care2Cure Cardiology v3"} {
				healthcareService.Name = new(name)
				require.NoError(t, source.Update("HealthcareService/"+*healthcareService.Id, healthcareService, nil))
			}

			report := sync(t, h.KnooppuntInternalBaseURL)

			assertReport(t, report, 0, 1)
			local := readLocalCopy[fhir.HealthcareService](t, h, query, "HealthcareService", *healthcareService.Id)
			assert.Equal(t, "Care2Cure Cardiology v3", *local.Name)
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}

// putLocations adds locationCount Locations managed by the given organization
// to the Central Addressing Directory, in one transaction.
func putLocations(t *testing.T, source fhirclient.Client, organizationID string) {
	t.Helper()
	tx := fhir.Bundle{Type: fhir.BundleTypeTransaction}
	for i := range locationCount {
		id := locationID(i)
		resource, err := json.Marshal(fhir.Location{
			Id:                   new(id),
			Identifier:           []fhir.Identifier{{System: new(testIdentifierSystem), Value: new(id)}},
			Name:                 new(fmt.Sprintf("Care2Cure location %d", i)),
			ManagingOrganization: &fhir.Reference{Reference: new("Organization/" + organizationID)},
		})
		require.NoError(t, err)
		tx.Entry = append(tx.Entry, fhir.BundleEntry{
			Resource: resource,
			Request:  &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT, Url: "Location/" + id},
		})
	}
	require.NoError(t, source.CreateWithContext(t.Context(), tx, nil, fhirclient.AtPath("/")))
}

func locationID(i int) string {
	return fmt.Sprintf("lrza-e2e-location-%03d", i)
}

// sync triggers one LRZA sync cycle and returns its report.
func sync(t *testing.T, internalBaseURL *url.URL) api.DirectoryUpdateReport {
	t.Helper()
	resp, err := http.Post(internalBaseURL.JoinPath("lrza/update").String(), "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "LRZA sync failed: %s", body)

	var report api.DirectoryUpdateReport
	require.NoError(t, json.Unmarshal(body, &report))
	return report
}

// assertReport checks a sync report's counts, and that it carries no warnings
// or errors. Nothing is ever deleted: withdrawal is a status change.
func assertReport(t *testing.T, report api.DirectoryUpdateReport, created, updated int) {
	t.Helper()
	assert.Equal(t, created, *report.Created, "created")
	assert.Equal(t, updated, *report.Updated, "updated")
	assert.Equal(t, 0, *report.Deleted, "deleted")
	assert.Empty(t, *report.Warnings, "warnings")
	assert.Empty(t, *report.Errors, "errors")
}

// sourceURL is the meta.source the update client gives the local copy of a
// Central Addressing Directory resource.
func sourceURL(h harness.Details, resourceType, id string) string {
	return h.LRZaFHIRBaseURL.JoinPath(resourceType, id).String()
}

// readLocalCopy finds the Local Query Directory's copy of a Central Addressing
// Directory resource by its meta.source.
func readLocalCopy[T any](t *testing.T, h harness.Details, query fhirclient.Client, resourceType, sourceID string) T {
	t.Helper()
	var bundle fhir.Bundle
	require.NoError(t, query.Search(resourceType, url.Values{"_source": []string{sourceURL(h, resourceType, sourceID)}}, &bundle))
	require.Len(t, bundle.Entry, 1, "expected exactly one local copy of %s/%s", resourceType, sourceID)
	var resource T
	require.NoError(t, json.Unmarshal(bundle.Entry[0].Resource, &resource))
	return resource
}

func searchOrg(t *testing.T, client fhirclient.Client, ura string) fhir.Organization {
	t.Helper()
	var bundle fhir.Bundle
	require.NoError(t, client.Search("Organization", url.Values{"identifier": []string{coding.URANamingSystem + "|" + ura}}, &bundle))
	require.Len(t, bundle.Entry, 1, "expected exactly one organization with URA %s", ura)
	var organization fhir.Organization
	require.NoError(t, json.Unmarshal(bundle.Entry[0].Resource, &organization))
	return organization
}

// countByIdentifierSystem counts the resources of a type this test added.
func countByIdentifierSystem(t *testing.T, client fhirclient.Client, resourceType string) int {
	t.Helper()
	var bundle fhir.Bundle
	require.NoError(t, client.Search(resourceType, url.Values{
		"identifier": []string{testIdentifierSystem + "|"},
		"_summary":   []string{"count"},
	}, &bundle))
	require.NotNil(t, bundle.Total)
	return *bundle.Total
}
