// Package seed contains end-to-end tests for the GF Sandbox canonical dataset
// (issue #542, epic E5): the Plataan organization, the demo-patient pool, the
// NVI localization registrations, and the reset/recycle paths.
package seed

import (
	"bytes"
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

func zonnebloemURA() string { return *sunflower.Organization().Identifier[0].Value }

// sharePlataanAs publishes De Plataan's localization records for one patient
// under clientID, which is what a demo run does and what "restored to the seeded
// fixtures" has to undo, and requires the share to be visible before returning.
func sharePlataanAs(t *testing.T, h harness.Details, p pool.PoolPatient, clientID string) {
	t.Helper()
	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")
	require.NoError(t, nvi.Register(t.Context(), nviBaseURL, nvi.Registration{
		CustodianURA: plataan.URA, BSN: p.BSN, ClientID: clientID, Categories: p.PlataanCategories(),
	}))
	before, err := nvi.ListsForCustodian(t.Context(), nviBaseURL, plataan.URA, p.BSN)
	require.NoError(t, err)
	require.NotEmptyf(t, before, "precondition: %s is shared under client %s", p.Key, clientID)
}

// sharePlataan is sharePlataanAs under the client id the sandbox publishes with.
func sharePlataan(t *testing.T, h harness.Details, p pool.PoolPatient) {
	t.Helper()
	sharePlataanAs(t, h, p, pool.PlataanClientID)
}

// requireUnshared asserts the patient went back to the pool unshared.
func requireUnshared(t *testing.T, h harness.Details, p pool.PoolPatient) {
	t.Helper()
	after, err := nvi.ListsForCustodian(t.Context(), h.KnooppuntInternalBaseURL.JoinPath("nvi"),
		plataan.URA, p.BSN)
	require.NoError(t, err)
	require.Emptyf(t, after, "the notice says %s was restored, so the share must be gone", p.Key)
}

// requireZonnebloemSeeded asserts the source side holds exactly the seeded
// registration, which E4 depends on.
func requireZonnebloemSeeded(t *testing.T, h harness.Details, p pool.PoolPatient, msgAndArgs ...any) {
	t.Helper()
	lists, err := nvi.ListsForCustodian(t.Context(), h.KnooppuntInternalBaseURL.JoinPath("nvi"),
		zonnebloemURA(), p.BSN)
	require.NoError(t, err)
	if len(msgAndArgs) == 0 {
		msgAndArgs = []any{"the source side must hold %s's seeded registration", p.Key}
	}
	require.Equal(t, p.ZonnebloemCategories(), nvi.CategoriesOf(lists), msgAndArgs...)
}

// seededAllergyID is the fixed id of the patient's seeded De Zonnebloem allergy.
// Pool resource ids are deterministic: pool-<key>-<side>-<suffix>.
func seededAllergyID(p pool.PoolPatient) string { return "pool-" + p.Key + "-zonnebloem-allergy" }

// deleteSeededAllergy removes one of the patient's seeded De Zonnebloem fixtures,
// so that "restored" afterwards means something a no-op cannot satisfy. A leaf
// resource, because HAPI refuses to delete the Patient while its children still
// reference it.
func deleteSeededAllergy(t *testing.T, h harness.Details, p pool.PoolPatient) {
	t.Helper()
	zonnebloem := sunflower.PatientsHAPITenant().FHIRClient(h.HAPIBaseURL)
	require.NoError(t, zonnebloem.DeleteWithContext(t.Context(), "AllergyIntolerance/"+seededAllergyID(p)))
}

// requireSeededAllergyRestored asserts the fixture deleteSeededAllergy removed is
// back, by its fixed id.
func requireSeededAllergyRestored(t *testing.T, h harness.Details, p pool.PoolPatient, msgAndArgs ...any) {
	t.Helper()
	var allergy fhir.AllergyIntolerance
	err := sunflower.PatientsHAPITenant().FHIRClient(h.HAPIBaseURL).ReadWithContext(
		t.Context(), "AllergyIntolerance/"+seededAllergyID(p), &allergy)
	if len(msgAndArgs) == 0 {
		msgAndArgs = []any{"%s's seeded allergy must be restored", p.Key}
	}
	require.NoError(t, err, msgAndArgs...)
}

// sandboxTarget points reset and recycle at the harness, mock Mitz included.
// Leaving that URL nil would exercise the unconfigured path in every test that
// only wants a working reset, and the unconfigured path is now a warning.
func sandboxTarget(t *testing.T, h harness.Details) vectors.SandboxTarget {
	t.Helper()
	mockURL, err := url.Parse(h.MockMitzXACML.GetURL())
	require.NoError(t, err)
	return vectors.SandboxTarget{
		HAPIBaseURL:              h.HAPIBaseURL,
		KnooppuntInternalBaseURL: h.KnooppuntInternalBaseURL,
		MitzMockBaseURL:          mockURL,
	}
}

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

// The pool seeds De Zonnebloem's side only: it is the source holding the BGZ.
// De Plataan's side is published by the sandbox when a patient is shared, so a
// freshly seeded patient must have nothing under 00000010 (DESIGN §5.7).
//
// Both halves are asserted. Checking only the absence would let a seed that
// registers nothing at all pass, silently removing E4's prerequisite.
func TestSeed_ZonnebloemRegisteredPlataanNot(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")

	for _, p := range pool.Patients() {
		zonnebloem, err := nvi.ListsForCustodian(t.Context(), nviBaseURL, zonnebloemURA(), p.BSN)
		require.NoError(t, err)
		require.Equal(t, p.ZonnebloemCategories(), nvi.CategoriesOf(zonnebloem),
			"patient %s should be registered for exactly the categories De Zonnebloem holds", p.Key)

		plataanLists, err := nvi.ListsForCustodian(t.Context(), nviBaseURL, plataan.URA, p.BSN)
		require.NoError(t, err)
		require.Empty(t, plataanLists, "patient %s must start unshared from De Plataan", p.Key)
	}
}

func TestSeed_NVIIsIdempotent(t *testing.T) {
	h := harness.Start(t)

	// Seed twice; delete-then-create must not accumulate duplicates.
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")
	for _, p := range pool.Patients() {
		zonnebloem, err := nvi.ListsForCustodian(t.Context(), nviBaseURL, zonnebloemURA(), p.BSN)
		require.NoError(t, err)
		require.Lenf(t, zonnebloem, len(p.ZonnebloemCategories()),
			"patient %s should have one List per category after a double seed, not two", p.Key)

		plataanLists, err := nvi.ListsForCustodian(t.Context(), nviBaseURL, plataan.URA, p.BSN)
		require.NoError(t, err)
		require.Emptyf(t, plataanLists, "patient %s is not seeded from De Plataan's side", p.Key)
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

	require.NoError(t, vectors.ResetGlobal(t.Context(), sandboxTarget(t, h)))

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
	deleteSeededAllergy(t, h, anna)

	require.NoError(t, vectors.ResetGlobal(t.Context(), sandboxTarget(t, h)))

	// The user-created record is gone (cascaded away with the Patient it hung off)...
	var gone fhir.Procedure
	err := zonnebloem.ReadWithContext(t.Context(), "Procedure/"+*created.Id, &gone)
	require.Error(t, err, "user-created marker %s should be removed by ResetGlobal", *created.Id)

	// ...and the deleted fixture is restored, along with the pool patient itself.
	requireSeededAllergyRestored(t, h, anna)
	var patient fhir.Patient
	require.NoError(t, zonnebloem.ReadWithContext(t.Context(), "Patient/"+anna.ZonnebloemPatientID, &patient))
	require.Equal(t, anna.BSN, *patient.Identifier[0].Value)
}

// The cleanup runs after restoration and does not skip it: a warning about a
// mock nobody can reach must not leave the dataset unrestored.
func TestResetGlobal_RestoresEvenWhenMitzCleanupFails(t *testing.T) {
	h := harness.Start(t)
	unreachable, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)

	// A shared patient and a deleted fixture first. Asserting only an untouched
	// registration afterwards cannot tell restoration from a reset that skipped
	// it: a cleanup returning its warning between clearing the tenants and
	// reloading them empties both stores and leaves that registration exactly
	// where it was.
	anna := pool.Patients()[0]
	sharePlataan(t, h, anna)
	deleteSeededAllergy(t, h, anna)

	target := sandboxTarget(t, h)
	target.MitzMockBaseURL = unreachable
	err = vectors.ResetGlobal(t.Context(), target)

	require.ErrorIs(t, err, vectors.ErrPartialReset)

	// Both halves of "restored", the reload and the unshare, because they are
	// separate steps and a cleanup that warned between them satisfies whichever
	// one is left unasserted.
	requireSeededAllergyRestored(t, h, anna, "the cleanup warning must not stand in for a dataset that was never restored")
	requireUnshared(t, h, anna)
	requireZonnebloemSeeded(t, h, anna, "the fixtures must be restored even when the optional cleanup warned")
}

// A reset must return every patient to the pool unshared, which is what its own
// notice tells the presenter it did. Nothing covered this before: the reset tests
// ran against a harness where nothing was ever shared, and the seed's earlier
// custodian-wide delete cleaned up De Plataan's side as a side effect, which
// narrowing that delete to the registering client removed.
func TestResetGlobal_UnsharesAPatientSharedDuringADemo(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	// Two patients, not one: a fix that unshared only the patient it was handed
	// would satisfy a single-patient assertion.
	for _, p := range pool.Patients()[:2] {
		sharePlataan(t, h, p)
	}

	require.NoError(t, vectors.ResetGlobal(t.Context(), sandboxTarget(t, h)))

	for _, p := range pool.Patients() {
		requireUnshared(t, h, p)
		// And the source side is still there, or the reset broke what E4 needs.
		requireZonnebloemSeeded(t, h, p)
	}
}

// The other half of "cleanup runs last": its warning must not stand in for a
// real failure above it. The test above cannot see this: an implementation that
// ran the cleanup first, kept its error, then restored while swallowing the
// restore errors, and returned the kept error would satisfy both of its
// assertions. Here restoration itself is broken, so a reset that returns
// ErrPartialReset is reporting a mock it could not reach over a dataset it could
// not restore.
func TestResetGlobal_ACleanupWarningDoesNotMaskARestoreFailure(t *testing.T) {
	h := harness.Start(t)
	unreachableMitz, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)
	unreachableHAPI, err := url.Parse("http://127.0.0.1:2/fhir")
	require.NoError(t, err)

	target := sandboxTarget(t, h)
	target.HAPIBaseURL, target.MitzMockBaseURL = unreachableHAPI, unreachableMitz
	err = vectors.ResetGlobal(t.Context(), target)

	require.Error(t, err, "a reset that cannot reach HAPI has failed, not warned")
	require.NotErrorIs(t, err, vectors.ErrPartialReset,
		"the restoration failure must surface as itself, not as a cleanup warning")
}

// A registration this cleanup cannot match must not be reported as a clean
// restore. The branch's own earlier seed wrote De Plataan under a different
// identifier system, and Compose reuses an unchanged HAPI container, so an
// upgraded stack still holds records the client-scoped delete cannot see.
func TestResetGlobal_ReportsPartialWhenAForeignClientRegistrationSurvives(t *testing.T) {
	h := harness.Start(t)
	anna := pool.Patients()[0]
	// Another client's registration for the same custodian and patient.
	sharePlataanAs(t, h, anna, "PLATAAN-EHR-legacy")

	err := vectors.ResetGlobal(t.Context(), sandboxTarget(t, h))

	require.ErrorIs(t, err, vectors.ErrPartialReset,
		"a patient still findable under De Plataan is not a restored dataset")
	require.Contains(t, err.Error(), anna.Key)
	// The rest of the restore still happened: the warning is not an early exit.
	requireZonnebloemSeeded(t, h, anna)
}

// The same for one patient.
func TestRecyclePatient_ReportsPartialWhenAForeignClientRegistrationSurvives(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	anna := pool.Patients()[0]
	sharePlataanAs(t, h, anna, "PLATAAN-EHR-legacy")

	err := vectors.RecyclePatient(t.Context(), sandboxTarget(t, h), anna.Key)

	require.ErrorIs(t, err, vectors.ErrPartialReset)
	require.Contains(t, err.Error(), anna.Key)
}

// The sandbox publishes under SANDBOX_NVI_CLIENT_ID when it is set, and the
// delete that cleans up is scoped to a client. A reset holding the compiled-in
// default while the app published under an override removes nothing and still
// reports that the dataset was restored, so the id has to travel with the target.
func TestResetGlobal_UnsharesUnderTheConfiguredClientID(t *testing.T) {
	h := harness.Start(t)
	const overrideClientID = "gf-sandbox-plataan-override"
	anna := pool.Patients()[0]
	sharePlataanAs(t, h, anna, overrideClientID)

	target := sandboxTarget(t, h)
	target.NVIClientID = overrideClientID
	require.NoError(t, vectors.ResetGlobal(t.Context(), target))

	requireUnshared(t, h, anna)
}

// Reset without a mock Mitz cannot clear consent subscriptions, and must say so:
// the base compose profile runs the sandbox with MITZMOCK_URL unset while the
// subscription still reaches Mitz through the Knooppunt, so a silent success is
// the cleanup button reporting a clean consent state it never touched.
func TestResetGlobal_WithoutAMockReportsPartialNotClean(t *testing.T) {
	h := harness.Start(t)
	target := sandboxTarget(t, h)
	target.MitzMockBaseURL = nil

	// Shared and with a fixture deleted first, so "restored" means something a
	// nil-only early return cannot satisfy.
	anna := pool.Patients()[0]
	sharePlataan(t, h, anna)
	deleteSeededAllergy(t, h, anna)

	err := vectors.ResetGlobal(t.Context(), target)

	require.ErrorIs(t, err, vectors.ErrPartialReset,
		"a reset that could not clear subscriptions must not report a clean dataset")

	// And the dataset was still restored: the warning is about the mock, not a
	// reason to skip the work.
	requireSeededAllergyRestored(t, h, anna)
	requireUnshared(t, h, anna)
	requireZonnebloemSeeded(t, h, anna)
}

// The same for recycle: its cleanup runs after the restore steps so a warning
// cannot stand in for a patient that was never restored, and that ordering has to
// be pinned or moving the cleanup to the front leaves every recycle test green.
func TestRecyclePatient_ACleanupWarningDoesNotMaskAFailedRestore(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	unreachable, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)

	anna := pool.Patients()[0]
	sharePlataan(t, h, anna)
	deleteSeededAllergy(t, h, anna)

	target := sandboxTarget(t, h)
	target.MitzMockBaseURL = unreachable
	err = vectors.RecyclePatient(t.Context(), target, anna.Key)

	require.ErrorIs(t, err, vectors.ErrPartialReset,
		"an uncleared subscription is a warning, not a failed recycle")
	requireSeededAllergyRestored(t, h, anna, "the warning must not stand in for a patient that was never restored")
	requireUnshared(t, h, anna)
}

// The client id has to reach recycle too, not only the global reset.
func TestRecyclePatient_UnsharesUnderTheConfiguredClientID(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))
	const overrideClientID = "gf-sandbox-plataan-override"
	anna := pool.Patients()[0]
	sharePlataanAs(t, h, anna, overrideClientID)

	target := sandboxTarget(t, h)
	target.NVIClientID = overrideClientID
	require.NoError(t, vectors.RecyclePatient(t.Context(), target, anna.Key))

	requireUnshared(t, h, anna)
}

// A recycled patient goes back to the pool unshared, and the consent subscription
// is part of that: left behind, the next share finds it instead of creating one.
// Scoped, because a recycle must not disturb a concurrent demo.
func TestRecyclePatient_ClearsOnlyThatPatientsSubscription(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	patients := pool.Patients()
	recycled, other := patients[0], patients[1]
	for _, p := range []pool.PoolPatient{recycled, other} {
		subscribeAtMock(t, h, p.BSN)
	}
	require.Len(t, h.MockMitzXACML.GetSubscriptions(), 2, "precondition: both patients are subscribed")

	require.NoError(t, vectors.RecyclePatient(t.Context(), sandboxTarget(t, h), recycled.Key))

	remaining := h.MockMitzXACML.GetSubscriptions()
	require.Len(t, remaining, 1, "recycle must clear the recycled patient's subscription")
	require.Contains(t, remaining[0].Criteria, "patientid="+other.BSN,
		"and must leave a concurrent demo's subscription alone")
}

// subscribeAtMock registers a De Plataan consent subscription straight at the
// mock, which is what the share flow ends up doing through the Knooppunt.
func subscribeAtMock(t *testing.T, h harness.Details, bsn string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"resourceType": "Subscription",
		"status":       "requested",
		"reason":       "OTV",
		"criteria": "Consent?_query=otv&patientid=" + bsn +
			"&providerid=" + plataan.URA + "&providertype=Z3",
		"channel": map[string]any{"type": "rest-hook"},
	})
	require.NoError(t, err)
	res, err := http.Post(h.MockMitzXACML.GetURL()+"/abonnementen/fhir/Subscription",
		"application/fhir+json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
}

func TestRecyclePatient_RestoresTargetLeavesOthersIntact(t *testing.T) {
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	patients := pool.Patients()
	target, other := patients[0], patients[1]
	nviBaseURL := h.KnooppuntInternalBaseURL.JoinPath("nvi")

	// Simulate demo drift: remove the target's seeded Zonnebloem registration.
	// De Plataan's no longer exists to delete, since the pool no longer seeds it.
	require.NoError(t, nvi.DeleteForClient(t.Context(), nviBaseURL, zonnebloemURA(), target.BSN, pool.ZonnebloemClientID))
	before, err := nvi.ListsForCustodian(t.Context(), nviBaseURL, zonnebloemURA(), target.BSN)
	require.NoError(t, err)
	require.Empty(t, before, "precondition: the target's Zonnebloem registration is gone")

	require.NoError(t, vectors.RecyclePatient(t.Context(), sandboxTarget(t, h), target.Key))

	// The target is restored on the source side and unshared on Plataan's...
	requireZonnebloemSeeded(t, h, target)
	requireUnshared(t, h, target)

	// ...and another patient is untouched.
	requireZonnebloemSeeded(t, h, other)

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
