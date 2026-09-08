package vectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/care2cure"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pip"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type KnooppuntSystemDetails struct {
	MCSD KnooppuntMCSDDetails
}

type FHIRAPIDetails struct {
	FHIRBaseURL *url.URL
}

type KnooppuntMCSDDetails struct {
	AdminFHIRBaseURL *url.URL
	QueryFHIRBaseURL *url.URL
}

type Details struct {
	Knooppunt KnooppuntSystemDetails
	LRZa      FHIRAPIDetails
	NVI       FHIRAPIDetails
	PIP       FHIRAPIDetails
}

// allTenants returns every HAPI tenant the seed manages, in creation order.
func allTenants() []hapi.Tenant {
	return []hapi.Tenant{
		{Name: "knpt-mcsd-query", ID: 2},
		{Name: "knpt-mcsd-admin", ID: 1},
		lrza.HAPITenant(),
		care2cure.AdminHAPITenant(),
		sunflower.AdminHAPITenant(),
		sunflower.PatientsHAPITenant(),
		nvi.HAPITenant(),
		pip.HAPITenant(),
		plataan.AdminHAPITenant(),
		plataan.PatientsHAPITenant(),
	}
}

// Load seeds all FHIR test data into HAPI: the mCSD directories (LRZa root plus
// each organization's admin directory), the PIP, and the demo-patient pool's
// clinical resources on both the Plataan and Zonnebloem sides.
//
// Load is idempotent: every write is a PUT-by-fixed-id upsert and there is no
// destructive $expunge, so re-running Load (first boot, re-seed, reset) restores
// the seeded resources without touching resources it does not own. It does not
// register NVI Lists — those go through the Knooppunt; call SeedNVI for that.
func Load(hapiBaseURL *url.URL) (*Details, error) {
	ctx := context.Background()

	knptMCSDAdminHAPITenant := hapi.Tenant{Name: "knpt-mcsd-admin", ID: 1}
	knptMCSDQueryHAPITenant := hapi.Tenant{Name: "knpt-mcsd-query", ID: 2}
	lrzaMCSDAdminHAPITenant := lrza.HAPITenant()
	nviTenant := nvi.HAPITenant()
	pipTenant := pip.HAPITenant()

	hapiDefaultFHIRClient := fhirclient.New(hapiBaseURL, http.DefaultClient, nil)

	// Create tenants (idempotent: an "already exists" partition error is ignored).
	for _, tenant := range allTenants() {
		if err := hapi.CreateTenant(ctx, tenant, hapiDefaultFHIRClient); err != nil {
			return nil, fmt.Errorf("create HAPI tenant: %w", err)
		}
	}

	//
	// Knooppunt mCSD Admin (LRZa root directory)
	//
	if err := putResources(ctx, lrzaMCSDAdminHAPITenant.FHIRClient(hapiBaseURL), lrza.Resources(hapiBaseURL)); err != nil {
		return nil, fmt.Errorf("seed LRZa resources: %w", err)
	}

	//
	// Care2Cure Hospital
	//
	if err := putResources(ctx, care2cure.AdminHAPITenant().FHIRClient(hapiBaseURL), care2cure.Resources()); err != nil {
		return nil, fmt.Errorf("seed care2cure resources: %w", err)
	}

	//
	// Sunflower / Zonnebloem Care Home (source of the BGZ)
	//
	if err := putResources(ctx, sunflower.AdminHAPITenant().FHIRClient(hapiBaseURL), sunflower.AdminResources()); err != nil {
		return nil, fmt.Errorf("seed sunflower admin resources: %w", err)
	}
	if err := putResources(ctx, sunflower.PatientsHAPITenant().FHIRClient(hapiBaseURL), sunflower.PatientsResources()); err != nil {
		return nil, fmt.Errorf("seed sunflower patients resources: %w", err)
	}

	//
	// Plataan Hospital (consumer/requester)
	//
	if err := putResources(ctx, plataan.AdminHAPITenant().FHIRClient(hapiBaseURL), plataan.AdminResources()); err != nil {
		return nil, fmt.Errorf("seed plataan admin resources: %w", err)
	}

	//
	// Demo patient pool: Plataan-side data in the plataan-patients tenant,
	// Zonnebloem-side data in the existing sunflower-patients tenant.
	//
	if err := putResources(ctx, plataan.PatientsHAPITenant().FHIRClient(hapiBaseURL), pool.PlataanResources()); err != nil {
		return nil, fmt.Errorf("seed pool plataan resources: %w", err)
	}
	if err := putResources(ctx, sunflower.PatientsHAPITenant().FHIRClient(hapiBaseURL), pool.ZonnebloemResources()); err != nil {
		return nil, fmt.Errorf("seed pool zonnebloem resources: %w", err)
	}

	//
	// Policy information point
	//
	if err := putResources(ctx, pipTenant.FHIRClient(hapiBaseURL), pip.Resources(hapiBaseURL)); err != nil {
		return nil, fmt.Errorf("seed pip resources: %w", err)
	}

	return &Details{
		Knooppunt: KnooppuntSystemDetails{
			MCSD: KnooppuntMCSDDetails{
				AdminFHIRBaseURL: knptMCSDAdminHAPITenant.BaseURL(hapiBaseURL),
				QueryFHIRBaseURL: knptMCSDQueryHAPITenant.BaseURL(hapiBaseURL),
			},
		},
		LRZa: FHIRAPIDetails{FHIRBaseURL: lrzaMCSDAdminHAPITenant.BaseURL(hapiBaseURL)},
		NVI:  FHIRAPIDetails{FHIRBaseURL: nviTenant.BaseURL(hapiBaseURL)},
		PIP:  FHIRAPIDetails{FHIRBaseURL: pipTenant.BaseURL(hapiBaseURL)},
	}, nil
}

// ExpungeAll removes ALL data from the HAPI server (system-level $expunge),
// every tenant, not just the ones this package seeds.
//
// DANGER: this is unconditionally destructive and is NOT part of the normal seed
// path (Load is non-destructive and idempotent). It exists for the e2e test
// harness, which reuses HAPI containers and needs a clean store between tests.
// Never point it at a HAPI instance whose contents you care about; for restoring
// the demo dataset use ResetGlobal, which expunges only the mutable tenants.
func ExpungeAll(hapiBaseURL *url.URL) error {
	client := fhirclient.New(hapiBaseURL, http.DefaultClient, nil)
	return client.CreateWithContext(context.Background(), fhir.Parameters{
		Parameter: []fhir.ParametersParameter{
			{
				Name:         "expungeEverything",
				ValueBoolean: to.Ptr(true),
			},
		},
	}, nil, fhirclient.AtPath("/$expunge"))
}

// putResources PUT-upserts each resource by its fixed id into the given tenant.
func putResources(ctx context.Context, client fhirclient.Client, resources []fhir.HasId) error {
	for _, resource := range resources {
		path := caramel.ResourceType(resource) + "/" + *resource.GetId()
		if err := client.UpdateWithContext(ctx, path, resource, nil); err != nil {
			return fmt.Errorf("upsert %s: %w", path, err)
		}
	}
	return nil
}

// SeedNVI registers every pool patient's NVI localization Lists through the
// Knooppunt's internal /nvi endpoint: one List per data category De Zonnebloem
// holds, and nothing on De Plataan's side, which the sandbox publishes when a
// patient is shared. It is idempotent: each registration is a delete-then-create
// per subject+custodian+client, so running SeedNVI repeatedly yields exactly one
// List per patient per custodian per category.
//
// The client is part of the key on purpose. A custodian-wide delete would erase
// records another installation of the same organization registered; the cost is
// that this function cannot clean up the sandbox's own registrations, which
// ResetGlobal does by name.
//
// knooppuntInternalBaseURL is the Knooppunt internal API base (e.g.
// http://knooppunt:8081); the /nvi path is appended here.
func SeedNVI(ctx context.Context, knooppuntInternalBaseURL *url.URL) error {
	nviBaseURL := knooppuntInternalBaseURL.JoinPath("nvi")
	for _, patient := range pool.Patients() {
		for _, reg := range patient.NVIRegistrations() {
			if err := nvi.Register(ctx, nviBaseURL, reg); err != nil {
				return fmt.Errorf("seed NVI for patient %s: %w", patient.Key, err)
			}
		}
	}
	return nil
}

// SandboxTarget names the stack a reset or recycle acts on, and the identity it
// acts as.
//
// The client id travels with the URLs on purpose. The sandbox publishes its
// localization records under a configurable client (SANDBOX_NVI_CLIENT_ID), and
// the delete that cleans them up is scoped to that client. A cleanup holding a
// different id removes nothing and reports that it restored the dataset, which
// is the failure this struct exists to make hard: one value, passed by the
// caller that owns it, rather than a constant each side reaches for separately.
type SandboxTarget struct {
	HAPIBaseURL              *url.URL
	KnooppuntInternalBaseURL *url.URL

	// MitzMockBaseURL is nil when no mock is configured. Cleanup then cannot run
	// and the caller is told so; see clearMitzSubscriptions.
	MitzMockBaseURL *url.URL

	// NVIClientID is the client the sandbox publishes under. Empty means the
	// compiled-in default, which is what the sandbox share flow uses when
	// SANDBOX_NVI_CLIENT_ID is unset. Not the seed: SeedNVI publishes De
	// Zonnebloem's registrations, under pool.ZonnebloemClientID.
	NVIClientID string
}

func (t SandboxTarget) clientID() string {
	if t.NVIClientID == "" {
		return pool.PlataanClientID
	}
	return t.NVIClientID
}

// ResetGlobal restores the entire seeded dataset to its fixtures ("restore
// fixtures" path). It clears the mutable stores — removing any user-created
// records, which have random ids a plain re-seed cannot overwrite — then re-runs
// Load (re-PUTs the mCSD/PIP directories and pool resources) and SeedNVI, removes
// the localization records the sandbox published on De Plataan's side, and clears
// the mock Mitz's captured consent subscriptions. Every pool patient ends up
// unshared, which is the state a demo starts from.
//
// Clearing is per-resource deletion within the tenant, not $expunge; see
// clearTenant for why the expunge form cannot be used here.
//
// Known gap: NVI Lists registered during a demo under a BSN outside the pool
// survive a global reset. The NVI tenant's pseudonymization interceptor rejects
// any List search not scoped to a patient/subject/source, so they cannot be
// enumerated to be deleted, and SeedNVI's delete-then-create only covers the
// pool's own BSNs. DESIGN §5.6 wants those gone; doing so needs the Knooppunt to
// expose a custodian-scoped listing (or the seed to track what it registered).
//
// An unconfigured or unreachable mock Mitz yields ErrPartialReset rather than
// failing the whole reset or claiming a clean slate that does not exist; see
// clearMitzSubscriptions. ResetGlobal still does not reach the mCSD query
// directory cache, which is rebuilt by the mCSD update process instead.
func ResetGlobal(ctx context.Context, target SandboxTarget) error {
	hapiBaseURL := target.HAPIBaseURL
	knooppuntInternalBaseURL := target.KnooppuntInternalBaseURL
	// Clear the mutable patient stores so user-created (random-id) records go.
	//
	// The NVI tenant is NOT cleared this way: its pseudonymization interceptor
	// rejects a List search that is not scoped to a patient/subject/source, so
	// there is no way to enumerate "every List" in it. SeedNVI below restores De
	// Zonnebloem's registrations, and the loop after it removes the ones the
	// sandbox published; see the known limitation in test/testdata/README.md for
	// what still survives.
	for _, tenant := range []hapi.Tenant{
		sunflower.PatientsHAPITenant(),
		plataan.PatientsHAPITenant(),
	} {
		if err := clearTenant(ctx, tenant.FHIRClient(hapiBaseURL)); err != nil {
			return fmt.Errorf("clear tenant %s: %w", tenant.Name, err)
		}
	}

	if _, err := Load(hapiBaseURL); err != nil {
		return fmt.Errorf("reload fixtures: %w", err)
	}
	if err := SeedNVI(ctx, knooppuntInternalBaseURL); err != nil {
		return fmt.Errorf("reseed NVI: %w", err)
	}

	// Remove what the demo published on De Plataan's side, so every patient goes
	// back to the pool unshared, which is what this function's callers tell the
	// presenter it did.
	//
	// SeedNVI cannot do this. It registers De Zonnebloem only, and its delete is
	// scoped to the client that registers, so it never touches records another
	// installation wrote. That scoping is deliberate — a custodian-wide delete
	// would erase a second installation's registrations — but it means the
	// sandbox's own records survive unless they are removed here by name.
	nviBaseURL := knooppuntInternalBaseURL.JoinPath("nvi")
	var stillShared []string
	for _, patient := range pool.Patients() {
		if err := nvi.DeleteForClient(ctx, nviBaseURL, plataan.URA, patient.BSN, target.clientID()); err != nil {
			return fmt.Errorf("unshare patient %s: %w", patient.Key, err)
		}
		remaining, err := nvi.ListsForCustodian(ctx, nviBaseURL, plataan.URA, patient.BSN)
		if err != nil {
			return fmt.Errorf("verify patient %s is unshared: %w", patient.Key, err)
		}
		if len(remaining) > 0 {
			stillShared = append(stillShared, patient.Key)
		}
	}

	// Last, and its error returned last: a partial-cleanup warning must not mask
	// a real restoration failure above it.
	if err := clearMitzSubscriptions(ctx, target.MitzMockBaseURL, url.Values{}); err != nil {
		return err
	}
	return unsharedOrPartial(stillShared)
}

// unsharedOrPartial turns surviving De Plataan registrations into a warning
// rather than letting the caller report a clean restore over them.
//
// The delete is scoped by client id, and deliberately so: a custodian-wide
// delete would erase a second installation's registrations. The cost is that it
// cannot reach records this installation did not write, and there are two ways
// to have them. The branch's own earlier seed registered De Plataan under a
// different identifier system, and Compose reuses an unchanged HAPI container,
// so an upgraded stack still holds them. An operator who changes
// SANDBOX_NVI_CLIENT_ID moves what the delete matches and strands whatever the
// previous value wrote.
//
// Checking what is actually there costs one search per patient and needs no
// knowledge of the formats that produced them.
func unsharedOrPartial(stillShared []string) error {
	if len(stillShared) == 0 {
		return nil
	}
	return fmt.Errorf("%w: these patients are still registered under De Plataan by a client this cleanup does not match, so they remain findable: %s",
		ErrPartialReset, strings.Join(stillShared, ", "))
}

// ErrPartialReset reports that the dataset was restored but something optional
// could not be cleared. Callers surface it rather than treating the reset as
// clean or as failed: both readings are wrong.
var ErrPartialReset = errors.New("reset completed with warnings")

// clearMitzSubscriptions drops captured consent subscriptions from the mock
// Mitz, so a reset restores a clean consent state (DESIGN §5.6). An empty scope
// clears everything; providerid and patientid narrow it to one pair.
//
// A nil URL yields ErrPartialReset, not success. No mock configured means the
// subscriptions cannot be cleared, and a subscription can exist regardless: the
// sandbox subscribes through the Knooppunt, which reaches Mitz whether or not
// MITZMOCK_URL was ever set, and the base compose profile sets it nowhere. A
// silent nil there is the reset telling the presenter it restored a clean
// consent state over one it never touched. A configured but unreachable mock is
// the same answer for the same reason; failing the whole reset instead would make
// demo cleanup depend on a mock's availability.
func clearMitzSubscriptions(ctx context.Context, mitzMockBaseURL *url.URL, scope url.Values) error {
	if mitzMockBaseURL == nil {
		return fmt.Errorf("%w: no mock Mitz is configured, so consent subscriptions were not cleared", ErrPartialReset)
	}
	endpoint := mitzMockBaseURL.JoinPath("abonnementen", "fhir", "Subscription")
	if len(scope) > 0 {
		endpoint.RawQuery = scope.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: build Mitz cleanup request: %w", ErrPartialReset, err)
	}
	res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("%w: Mitz subscriptions were not cleared: %w", ErrPartialReset, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: Mitz cleanup returned %s", ErrPartialReset, res.Status)
	}
	return nil
}

// RecyclePatient restores a single pool patient to its seeded, unshared state
// without touching any other patient: it removes that patient's De Plataan NVI
// registration and consent subscription, re-registers De Zonnebloem's, and
// re-PUTs its seeded FHIR resources by fixed id.
//
// The subscription is part of that state, not an afterthought. Demo state is
// keyed per patient (DESIGN §5.7) and the consent subscription is one of those
// keys, so leaving it behind returns a patient to the pool who is unshared in the
// NVI and still subscribed at Mitz. Sharing them again then finds the existing
// subscription instead of creating one, and the "restored to the seeded state"
// notice was untrue when it was shown.
//
// Known limitation: RecyclePatient cannot remove that patient's user-created
// marker records (random ids) — only a global reset (expunge) clears those. For
// the common between-demos need this is enough: recycle restores the seeded
// state and clears the shared/registered flags.
func RecyclePatient(ctx context.Context, target SandboxTarget, patientKey string) error {
	hapiBaseURL := target.HAPIBaseURL
	patient, ok := pool.PatientByKey(patientKey)
	if !ok {
		return fmt.Errorf("unknown pool patient: %q", patientKey)
	}

	// Re-publish the seeded registrations (Zonnebloem's) and remove De Plataan's,
	// which exists only if this patient was shared during a demo. Recycling a
	// patient means returning them to the pool unshared.
	nviBaseURL := target.KnooppuntInternalBaseURL.JoinPath("nvi")
	if err := nvi.DeleteForClient(ctx, nviBaseURL, plataan.URA, patient.BSN, target.clientID()); err != nil {
		return fmt.Errorf("recycle: remove plataan NVI registration for %s: %w", patient.Key, err)
	}
	for _, reg := range patient.NVIRegistrations() {
		if err := nvi.Register(ctx, nviBaseURL, reg); err != nil {
			return fmt.Errorf("recycle NVI for patient %s: %w", patient.Key, err)
		}
	}

	// Re-PUT the seeded FHIR resources by fixed id.
	if err := putResources(ctx, plataan.PatientsHAPITenant().FHIRClient(hapiBaseURL), patient.PlataanResources()); err != nil {
		return fmt.Errorf("recycle plataan resources for patient %s: %w", patient.Key, err)
	}
	if err := putResources(ctx, sunflower.PatientsHAPITenant().FHIRClient(hapiBaseURL), patient.ZonnebloemResources()); err != nil {
		return fmt.Errorf("recycle zonnebloem resources for patient %s: %w", patient.Key, err)
	}

	// Last, and scoped to this patient: clearing the store would cancel a
	// concurrent demo's subscription, which is the one thing a per-patient
	// recycle promises not to do. Its error is returned last so a cleanup warning
	// cannot mask a real restoration failure above it.
	if err := clearMitzSubscriptions(ctx, target.MitzMockBaseURL, url.Values{
		"providerid": {plataan.URA}, "patientid": {patient.BSN},
	}); err != nil {
		return err
	}

	// And the same check the global reset makes, for the same reason: the delete
	// above is scoped by client, so it cannot reach a registration another client
	// wrote, and "restored to the seeded state" would be untrue over one.
	remaining, err := nvi.ListsForCustodian(ctx, nviBaseURL, plataan.URA, patient.BSN)
	if err != nil {
		return fmt.Errorf("verify patient %s is unshared: %w", patient.Key, err)
	}
	if len(remaining) > 0 {
		return unsharedOrPartial([]string{patient.Key})
	}
	return nil
}

// mutableResourceTypes are the resource types the reset paths clear from the
// mutable tenants. It covers the types the pool seeds on both sides, the NVI
// List, plus the types a demo can create (Observation, DocumentReference), so
// user-created records are removed and not just the seeded fixtures.
var mutableResourceTypes = []string{
	"AllergyIntolerance",
	"Condition",
	"DocumentReference",
	"List",
	"MedicationRequest",
	"Observation",
	"Patient",
}

// clearTenant removes all data in a single HAPI partition, leaving the partition
// itself intact.
//
// It deliberately does NOT use $expunge with expungeEverything: HAPI treats that
// parameter as server-wide regardless of the tenant in the request path, so it
// deletes every partition's data AND the PartitionEntity rows defining the
// partitions — leaving every tenant unresolvable ("Partition name ... is not
// valid") until the server is rebuilt. See TestResetGlobal_PreservesPartitions.
//
// The conditional-delete form (`?_lastUpdated=gt...&_expunge=true`) is correctly
// partition-scoped but asynchronous: it enqueues a Batch2 DELETE_EXPUNGE job that
// only runs on HAPI's job-maintenance schedule (60s by default), which is far too
// slow for a reset behind a UI button. So this searches each resource type and
// deletes by id, which is synchronous and partition-scoped.
func clearTenant(ctx context.Context, client fhirclient.Client) error {
	for _, resourceType := range mutableResourceTypes {
		ids, err := searchResourceIDs(ctx, client, resourceType)
		if err != nil {
			return fmt.Errorf("search %s: %w", resourceType, err)
		}
		for _, id := range ids {
			// _cascade=delete because HAPI refuses to delete a resource that is
			// still referenced: a demo can create records of types this list does
			// not know about (Observation.subject -> Patient being the common
			// one), and those would otherwise block the Patient delete and fail
			// the whole reset.
			if err := client.DeleteWithContext(ctx, resourceType+"/"+id,
				fhirclient.QueryParam("_cascade", "delete")); err != nil {
				return fmt.Errorf("delete %s/%s: %w", resourceType, id, err)
			}
		}
	}
	return nil
}

// searchResourceIDs returns the ids of every resource of the given type in the
// client's tenant, following pagination via the bundle's "next" link.
func searchResourceIDs(ctx context.Context, client fhirclient.Client, resourceType string) ([]string, error) {
	var ids []string
	var bundle fhir.Bundle
	if err := client.SearchWithContext(ctx, resourceType, url.Values{"_count": {"500"}}, &bundle); err != nil {
		return nil, err
	}
	for {
		for _, entry := range bundle.Entry {
			var resource struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(entry.Resource, &resource); err != nil {
				return nil, fmt.Errorf("parse search entry: %w", err)
			}
			if resource.ID != "" {
				ids = append(ids, resource.ID)
			}
		}

		next := bundleNextLink(bundle)
		if next == nil {
			return ids, nil
		}
		bundle = fhir.Bundle{}
		if err := client.ReadWithContext(ctx, "", &bundle, fhirclient.AtUrl(next)); err != nil {
			return nil, fmt.Errorf("follow next page: %w", err)
		}
	}
}

// bundleNextLink returns the bundle's "next" pagination link, or nil.
func bundleNextLink(bundle fhir.Bundle) *url.URL {
	for _, link := range bundle.Link {
		if link.Relation != "next" {
			continue
		}
		next, err := url.Parse(link.Url)
		if err != nil {
			return nil
		}
		return next
	}
	return nil
}
