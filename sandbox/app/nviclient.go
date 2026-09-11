package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// plataanURA is the custodian the sandbox publishes under. Taken from the seed
// rather than restated, so the sandbox cannot query one custodian while the seed
// registers and recycles another.
const plataanURA = plataan.URA

// nviCallTimeout bounds a single NVI read. The list issues one per patient, so a
// slow NVI degrades individual rows to unknown instead of hanging the page.
const nviCallTimeout = 5 * time.Second

// nviRegisterTimeout bounds the whole registration, which is not one call but a
// search, a delete per existing record and a create per category. Budgeting it
// at nviCallTimeout showed the red "may be incomplete" card over a working but
// cold stack.
const nviRegisterTimeout = 20 * time.Second

// shareStatus is what the UI knows about a patient's localization and Mitz
// consent subscription. NVIUnknown is a third state on purpose: "we could not
// ask" is not "not shared", and rendering the latter is a lie the presenter acts
// on. SubscriptionUnknown is the same for Mitz: distinct from Subscribed being
// false, which would invite a retry that could duplicate against a real Mitz.
type shareStatus struct {
	Shared     bool
	Categories []string

	// UnnamedRecords is a count, not a flag inferred from an empty category set:
	// recognized and unnamed records coexist, and the mixed case is where a
	// screen would describe a stranger's record as one of ours.
	UnnamedRecords int

	NVIUnknown          bool
	Subscribed          bool
	SubscriptionUnknown bool
}

// nviRecords is what the NVI holds for one patient at De Plataan.
//
// Count is not derivable from Categories: a List with a missing or foreign code
// is a record other providers can find and a category this build cannot name,
// and deriving "shared" from the categories reported such a patient as
// local-only while they were findable.
type nviRecords struct {
	Count      int
	Categories []string

	// Unnamed is counted by the NVI helper, not derived as Count minus the
	// number of categories: that set is deduplicated, so the subtraction reads a
	// second List of the same category as one whose category cannot be named.
	Unnamed int
}

func (c Config) patientShareStatus(ctx context.Context, bsn string) shareStatus {
	if c.nviLookup == nil {
		return shareStatus{NVIUnknown: true}
	}
	callCtx, cancel := context.WithTimeout(ctx, nviCallTimeout)
	records, err := c.nviLookup(callCtx, bsn)
	cancel()
	if err != nil {
		// Logged because the screen tells the presenter the log carries the
		// reason. The last four digits of the BSN are enough to match a row to a
		// line without recording a national identifier.
		slog.WarnContext(ctx, "NVI lookup failed; reporting the patient's share status as unknown",
			"patient", nvi.LastFourOfBSN(bsn), "error", err)
		return shareStatus{NVIUnknown: true}
	}

	status := shareStatus{
		Shared:         records.Count > 0,
		Categories:     records.Categories,
		UnnamedRecords: records.Unnamed,
	}
	if !status.Shared {
		// Nothing is registered, so there is nothing a subscription would
		// accompany; asking is noise.
		return status
	}
	if c.mitzSubscribed == nil {
		status.SubscriptionUnknown = true
		return status
	}
	mitzCtx, cancel := context.WithTimeout(ctx, mitzCallTimeout)
	defer cancel()
	subscribed, err := c.mitzSubscribed(mitzCtx, bsn)
	if err != nil {
		status.SubscriptionUnknown = true
		return status
	}
	status.Subscribed = subscribed
	return status
}

// nviFuncs builds the real NVI calls against the Knooppunt's internal API.
func nviFuncs(knooppuntInternalURL *url.URL, clientID string) (
	func(context.Context, string) (nviRecords, error),
	func(context.Context, string, []string) error,
) {
	base := knooppuntInternalURL.JoinPath("nvi")

	lookup := func(ctx context.Context, bsn string) (nviRecords, error) {
		lists, err := nvi.ListsForCustodian(ctx, base, plataanURA, bsn)
		if err != nil {
			return nviRecords{}, err
		}
		return nviRecords{
			Count: len(lists), Categories: nvi.CategoriesOf(lists), Unnamed: nvi.UnnamedListCount(lists),
		}, nil
	}
	register := func(ctx context.Context, bsn string, cats []string) error {
		return nvi.Register(ctx, base, nvi.Registration{
			CustodianURA: plataanURA, BSN: bsn, ClientID: clientID, Categories: cats,
		})
	}
	return lookup, register
}

var patientWriteMu sync.Map // patient key -> *sync.Mutex

// lockPatientWrites serializes registration for one patient within this process.
func lockPatientWrites(key string) (unlock func()) {
	value, _ := patientWriteMu.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// registerPatient publishes De Plataan's localization records. nvi.Register is
// search-delete-create and not atomic (the NVI exposes no PUT and no usable
// conditional operation), so a double submit must not interleave. The caller
// holds the patient write lock, taken in handleShare so that the Mitz step sits
// in the same critical section.
func (c Config) registerPatient(ctx context.Context, patient pool.PoolPatient) error {
	if c.nviRegister == nil {
		return fmt.Errorf("the sandbox is not wired to the Knooppunt in this environment")
	}
	ctx, cancel := context.WithTimeout(ctx, nviRegisterTimeout)
	defer cancel()
	return c.nviRegister(ctx, patient.BSN, patient.PlataanCategories())
}
