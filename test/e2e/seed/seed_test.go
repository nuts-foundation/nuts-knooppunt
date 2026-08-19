// Package seed contains end-to-end tests for the GF Sandbox canonical dataset
// (issue #542, epic E5): the Plataan organization, the demo-patient pool, the
// NVI localization registrations, and the reset/recycle paths.
package seed

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const bsnSystem = "http://fhir.nl/fhir/NamingSystem/bsn"

func zonnebloemURA() string { return *sunflower.Organization().Identifier[0].Value }

// TestPoolBSNsPassElfproef is the E5 elfproef acceptance: every seeded pool BSN
// passes the 11-proof and none collides with the Nictiz fixture 999911120. This
// lives in the root module so it can use lib/bsnutil across the module boundary.
func TestPoolBSNsPassElfproef(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range pool.Patients() {
		require.Truef(t, bsnutil.ValidElfproef(p.BSN), "pool BSN %s (patient %s) must pass the elfproef", p.BSN, p.Key)
		require.NotEqualf(t, "999911120", p.BSN, "pool BSN must not collide with the Nictiz fixture (patient %s)", p.Key)
		require.Falsef(t, seen[p.BSN], "pool BSN %s is duplicated", p.BSN)
		seen[p.BSN] = true
	}
	require.NotEmpty(t, pool.Patients())
}

func TestSeed_PlataanOrganizationInQueryDirectory(t *testing.T) {
	h := harness.Start(t)

	// Run the mCSD update so the LRZa root + admin directories flow into the
	// query directory.
	invokeMCSDUpdate(t, h.KnooppuntInternalBaseURL)

	queryClient := fhirclient.New(h.MCSDQueryFHIRBaseURL, http.DefaultClient, nil)
	org, err := searchOrg(queryClient, plataan.URA)
	require.NoError(t, err)
	require.NotNil(t, org, "Plataan organization (URA %s) should be in the query directory", plataan.URA)
	assert.Equal(t, "Ziekenhuis De Plataan", *org.Name)
}

func TestSeed_NVIListsFindableUnderBothCustodians(t *testing.T) {
	h := harness.Start(t)

	// The harness loads FHIR data but not NVI Lists; seed them now.
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	anna := pool.Patients()[0]
	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")

	for _, custodian := range []string{plataan.URA, zonnebloemURA()} {
		entries := searchNVIByBSN(t, nviBaseURL, custodian, anna.BSN)
		require.Lenf(t, entries, 1, "patient %s should have exactly one NVI List under custodian %s", anna.Key, custodian)
	}
}

func TestSeed_NVIIsIdempotent(t *testing.T) {
	h := harness.Start(t)

	// Seed twice; delete-then-create must not accumulate duplicates.
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")
	for _, p := range pool.Patients() {
		for _, custodian := range []string{plataan.URA, zonnebloemURA()} {
			count, err := nvi.CountLists(t.Context(), nviBaseURL, custodian, p.BSN)
			require.NoError(t, err)
			require.Equalf(t, 1, count, "patient %s custodian %s should have exactly one List after double-seed", p.Key, custodian)
		}
	}
}

// TestResetGlobal_PreservesPartitions is the regression guard for the reset path
// destroying HAPI's partition table.
//
// ResetGlobal used to clear each mutable tenant with $expunge and
// expungeEverything=true. HAPI applies that parameter server-wide regardless of
// the tenant in the request path: it deleted every partition's data plus the
// PartitionEntity rows themselves, so afterwards every tenant — including the
// read-only ones the reset must not touch — failed with `Partition name "..." is
// not valid` until the server was rebuilt from scratch.
//
// A single reset was enough to break a freshly seeded stack, and nothing caught
// it because the harness re-runs Load (which recreates the partitions) after
// every expunge.
func TestResetGlobal_PreservesPartitions(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	require.NoError(t, vectors.ResetGlobal(t.Context(), h.HAPIBaseURL, h.KnooppuntInternalBaseURL))

	// Every tenant must still resolve — both the mutable ones the reset clears
	// and the read-only ones it must leave alone.
	for _, tenant := range []hapi.Tenant{
		sunflower.PatientsHAPITenant(),
		plataan.PatientsHAPITenant(),
		nvi.HAPITenant(),
		sunflower.AdminHAPITenant(),
		plataan.AdminHAPITenant(),
		lrza.HAPITenant(),
	} {
		var bundle fhir.Bundle
		err := tenant.FHIRClient(h.HAPIBaseURL).SearchWithContext(t.Context(), "Patient", url.Values{"_count": {"1"}}, &bundle)
		require.NoErrorf(t, err, "tenant %s should still resolve after ResetGlobal", tenant.Name)
	}
}

// TestResetGlobal_RemovesUserCreatedRecordsAndRestoresFixtures covers the other
// half of the reset contract: records created during a demo have random ids that
// a fixed-id re-seed cannot overwrite, so the reset must delete them, while the
// seeded fixtures come back.
func TestResetGlobal_RemovesUserCreatedRecordsAndRestoresFixtures(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	zonnebloem := sunflower.PatientsHAPITenant().FHIRClient(h.HAPIBaseURL)
	anna := pool.Patients()[0]

	// Simulate demo drift: a user-created record (random id) plus a deleted fixture.
	// The marker references the pool Patient, as a record created during a real
	// demo would, and is deliberately of a type the reset does not enumerate
	// (Procedure). HAPI refuses to delete a still-referenced resource, so without
	// a cascading delete this blocks the Patient and fails the whole reset — the
	// exact 500 seen in docker compose.
	var created fhir.Procedure
	require.NoError(t, zonnebloem.CreateWithContext(t.Context(), fhir.Procedure{
		Status:  fhir.EventStatusCompleted,
		Code:    &fhir.CodeableConcept{Text: to.Ptr("user-created marker")},
		Subject: fhir.Reference{Reference: to.Ptr("Patient/" + anna.ZonnebloemPatientID)},
	}, &created))
	require.NotNil(t, created.Id, "precondition: the marker got a server-assigned id")

	// Delete a seeded leaf resource (HAPI refuses to delete the Patient itself
	// while its children still reference it).
	// Pool resource ids are deterministic: pool-<key>-<side>-<suffix>.
	allergyID := "pool-" + anna.Key + "-zonnebloem-allergy"
	require.NoError(t, zonnebloem.DeleteWithContext(t.Context(), "AllergyIntolerance/"+allergyID))

	require.NoError(t, vectors.ResetGlobal(t.Context(), h.HAPIBaseURL, h.KnooppuntInternalBaseURL))

	// The user-created record is gone (cascaded away with the Patient it hung off)...
	var gone fhir.Procedure
	err := zonnebloem.ReadWithContext(t.Context(), "Procedure/"+*created.Id, &gone)
	require.Error(t, err, "user-created marker %s should be removed by ResetGlobal", *created.Id)

	// ...and the deleted fixture is restored.
	var allergy fhir.AllergyIntolerance
	require.NoError(t, zonnebloem.ReadWithContext(t.Context(), "AllergyIntolerance/"+allergyID, &allergy))

	// ...along with the pool patient itself.
	var patient fhir.Patient
	require.NoError(t, zonnebloem.ReadWithContext(t.Context(), "Patient/"+anna.ZonnebloemPatientID, &patient))
	require.Equal(t, anna.BSN, *patient.Identifier[0].Value)
}

func TestRecyclePatient_RestoresTargetLeavesOthersIntact(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	patients := pool.Patients()
	target, other := patients[0], patients[1]
	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")

	// Simulate demo drift: delete the target's Lists under one custodian.
	deleteNVIByBSN(t, nviBaseURL, plataan.URA, target.BSN)
	require.Len(t, searchNVIByBSN(t, nviBaseURL, plataan.URA, target.BSN), 0, "precondition: target's plataan List removed")

	// Recycle the target patient.
	require.NoError(t, vectors.RecyclePatient(t.Context(), h.HAPIBaseURL, h.KnooppuntInternalBaseURL, target.Key))

	// The target is restored under both custodians...
	for _, custodian := range []string{plataan.URA, zonnebloemURA()} {
		require.Lenf(t, searchNVIByBSN(t, nviBaseURL, custodian, target.BSN), 1,
			"recycled patient %s should have exactly one List under custodian %s", target.Key, custodian)
	}

	// ...and another patient's Lists are untouched (still exactly one each).
	for _, custodian := range []string{plataan.URA, zonnebloemURA()} {
		require.Lenf(t, searchNVIByBSN(t, nviBaseURL, custodian, other.BSN), 1,
			"non-recycled patient %s should still have exactly one List under custodian %s", other.Key, custodian)
	}

	// The target's Plataan-side Patient resource is restored (by fixed id).
	plataanPatients := fhirclient.New(plataan.PatientsHAPITenant().BaseURL(h.HAPIBaseURL), http.DefaultClient, nil)
	var patient fhir.Patient
	require.NoError(t, plataanPatients.Read("Patient/"+target.PlataanPatientID, &patient))
	require.Equal(t, target.BSN, *patient.Identifier[0].Value)
}

// --- helpers ---

func invokeMCSDUpdate(t *testing.T, internalBaseURL *url.URL) {
	t.Helper()
	resp, err := http.Post(internalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func nviClient(nviBaseURL *url.URL) fhirclient.Client {
	return fhirclient.New(nviBaseURL, http.DefaultClient, nil)
}

func tenantHeader(custodianURA string) fhirclient.PreRequestOption {
	return fhirclient.RequestHeaders(http.Header{
		"X-Tenant-ID": {coding.URANamingSystem + "|" + custodianURA},
	})
}

func searchNVIByBSN(t *testing.T, nviBaseURL *url.URL, custodianURA, bsn string) []fhir.BundleEntry {
	t.Helper()
	var searchSet fhir.Bundle
	err := nviClient(nviBaseURL).Search("List", url.Values{
		"subject:identifier": {bsnSystem + "|" + bsn},
	}, &searchSet, tenantHeader(custodianURA))
	require.NoError(t, err)
	return searchSet.Entry
}

// deleteNVIByBSN removes a subject's NVI Lists under a custodian by searching
// then deleting each by id (the fake-NVI store rejects a conditional delete with
// the ":identifier" modifier, so we resolve ids first).
func deleteNVIByBSN(t *testing.T, nviBaseURL *url.URL, custodianURA, bsn string) {
	t.Helper()
	client := nviClient(nviBaseURL)
	for _, entry := range searchNVIByBSN(t, nviBaseURL, custodianURA, bsn) {
		var list fhir.List
		require.NoError(t, json.Unmarshal(entry.Resource, &list))
		require.NotNil(t, list.Id)
		require.NoError(t, client.DeleteWithContext(t.Context(), "List/"+*list.Id, tenantHeader(custodianURA)))
	}
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
