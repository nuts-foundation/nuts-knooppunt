package main

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// plataanURA is the requesting hospital, and the custodian under which the
// sandbox publishes localization records when a patient is shared.
//
// Derived rather than restated: the seed registers and recycles against
// plataan.URA, so a second literal here would desync silently and leave the
// sandbox querying one custodian while the seed wrote another.
const plataanURA = plataan.URA

// nviCallTimeout bounds a single NVI read. The list issues one per patient, so a
// slow NVI degrades individual rows to unknown instead of hanging the page.
const nviCallTimeout = 5 * time.Second

// nviRegisterTimeout bounds the whole registration, which is not one call: a
// search, then a delete per record already there, then a create per category.
// Budgeting that at nviCallTimeout would show the red "may be incomplete" card
// over a working but cold stack, which is the demo asserting a failure that did
// not happen.
const nviRegisterTimeout = 20 * time.Second

// shareStatus is what the UI knows about a patient's localization and Mitz
// consent subscription state. NVIUnknown is a third state on purpose: "we
// could not ask" is not "not shared", and rendering the latter would be a lie
// the presenter acts on. SubscriptionUnknown carries the same reasoning for
// Mitz: it is distinct from Subscribed being false, which would invite a
// retry that could duplicate against a real Mitz.
type shareStatus struct {
	Shared              bool
	Categories          []string
	NVIUnknown          bool
	Subscribed          bool
	SubscriptionUnknown bool
}

func (c Config) patientShareStatus(ctx context.Context, bsn string) shareStatus {
	if c.nviCategories == nil {
		return shareStatus{NVIUnknown: true}
	}
	categories, err := c.withTimeout(ctx, nviCallTimeout, func(callCtx context.Context) ([]string, error) {
		return c.nviCategories(callCtx, bsn)
	})
	if err != nil {
		return shareStatus{NVIUnknown: true}
	}

	status := shareStatus{Shared: len(categories) > 0, Categories: categories}
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

// withTimeout derives d from ctx rather than from whatever time another call
// left on it, so two independent remote calls in the same request (NVI, then
// Mitz) each get their own budget instead of the second inheriting the
// first's leftovers.
func (c Config) withTimeout(ctx context.Context, d time.Duration,
	call func(context.Context) ([]string, error)) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return call(ctx)
}

// nviFuncs builds the real NVI calls against the Knooppunt's internal API.
func nviFuncs(knooppuntInternalURL *url.URL, clientID string) (
	func(context.Context, string) ([]string, error),
	func(context.Context, string, []string) error,
) {
	base := knooppuntInternalURL.JoinPath("nvi")

	categories := func(ctx context.Context, bsn string) ([]string, error) {
		lists, err := nvi.ListsForCustodian(ctx, base, plataanURA, bsn)
		if err != nil {
			return nil, err
		}
		return nvi.CategoriesOf(lists), nil
	}
	register := func(ctx context.Context, bsn string, cats []string) error {
		return nvi.Register(ctx, base, nvi.Registration{
			CustodianURA: plataanURA, BSN: bsn, ClientID: clientID, Categories: cats,
		})
	}
	return categories, register
}

var patientWriteMu sync.Map // patient key -> *sync.Mutex

// lockPatientWrites serializes registration for one patient within this process.
func lockPatientWrites(key string) (unlock func()) {
	value, _ := patientWriteMu.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// registerPatient publishes De Plataan's localization records, serialized per
// patient.
//
// nvi.Register is search-delete-create and is not atomic: the NVI exposes no PUT
// and no usable conditional operation. The mutex makes a double submit within
// this process converge instead of interleaving. It does not order two sandbox
// processes; that limitation is stated in the design and the README.
func (c Config) registerPatient(ctx context.Context, patient pool.PoolPatient) error {
	if c.nviRegister == nil {
		return fmt.Errorf("the sandbox is not wired to the Knooppunt in this environment")
	}
	unlock := lockPatientWrites(patient.Key)
	defer unlock()

	ctx, cancel := context.WithTimeout(ctx, nviRegisterTimeout)
	defer cancel()
	return c.nviRegister(ctx, patient.BSN, patient.PlataanCategories())
}
