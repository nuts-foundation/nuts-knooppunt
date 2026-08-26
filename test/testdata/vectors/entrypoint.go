package vectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
// Knooppunt's internal /nvi endpoint (one List per custodian). It is idempotent:
// each registration is a delete-then-create per subject+custodian, so running
// SeedNVI repeatedly yields exactly one List per patient per custodian.
//
// knooppuntInternalBaseURL is the Knooppunt internal API base (e.g.
// http://knooppunt:8081); the /nvi path is appended here.
func SeedNVI(ctx context.Context, knooppuntInternalBaseURL *url.URL) error {
	nviBaseURL := knooppuntInternalBaseURL.JoinPath("nvi")
	for _, patient := range pool.Patients() {
		for _, list := range patient.NVILists() {
			if err := nvi.RegisterList(ctx, nviBaseURL, list); err != nil {
				return fmt.Errorf("seed NVI for patient %s: %w", patient.Key, err)
			}
		}
	}
	return nil
}

// ResetGlobal restores the entire seeded dataset to its fixtures ("restore
// fixtures" path). It clears the mutable stores — removing any user-created
// records, which have random ids a plain re-seed cannot overwrite — then re-runs
// Load (re-PUTs the mCSD/PIP directories and pool resources) and SeedNVI.
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
// It does not reset Mitz subscriptions (no standalone mitz service yet; see
// README) or the mCSD query directory cache (rebuilt by the mCSD update process).
func ResetGlobal(ctx context.Context, hapiBaseURL, knooppuntInternalBaseURL *url.URL) error {
	// Clear the mutable patient stores so user-created (random-id) records go.
	//
	// The NVI tenant is NOT cleared this way: its pseudonymization interceptor
	// rejects a List search that is not scoped to a patient/subject/source, so
	// there is no way to enumerate "every List" in it. SeedNVI below is
	// delete-then-create per pool BSN, which restores the pool's own Lists to
	// exactly one each; see the known limitation in test/testdata/README.md.
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
	return nil
}

// RecyclePatient restores a single pool patient to its seeded, unshared state
// without touching any other patient: it deletes that patient's NVI Lists (both
// custodians) and re-PUTs its seeded FHIR resources by fixed id.
//
// Known limitation: RecyclePatient cannot remove that patient's user-created
// marker records (random ids) — only a global reset (expunge) clears those. For
// the common between-demos need this is enough: recycle restores the seeded
// state and clears the shared/registered flags.
func RecyclePatient(ctx context.Context, hapiBaseURL, knooppuntInternalBaseURL *url.URL, patientKey string) error {
	patient, ok := pool.PatientByKey(patientKey)
	if !ok {
		return fmt.Errorf("unknown pool patient: %q", patientKey)
	}

	// Re-register the NVI Lists (delete-then-create is idempotent and also
	// removes any extra Lists accumulated during the demo).
	nviBaseURL := knooppuntInternalBaseURL.JoinPath("nvi")
	for _, list := range patient.NVILists() {
		if err := nvi.RegisterList(ctx, nviBaseURL, list); err != nil {
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
