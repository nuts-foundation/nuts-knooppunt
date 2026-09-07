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

// nviCallTimeout bounds a single NVI call. The list issues one per patient, so a
// slow NVI degrades individual rows to unknown instead of hanging the page.
const nviCallTimeout = 5 * time.Second

// shareStatus is what the UI knows about a patient's localization state.
// NVIUnknown is a third state on purpose: "we could not ask" is not "not
// shared", and rendering the latter would be a lie the presenter acts on.
type shareStatus struct {
	Shared     bool
	Categories []string
	NVIUnknown bool
}

func (c Config) patientShareStatus(ctx context.Context, bsn string) shareStatus {
	if c.nviCategories == nil {
		return shareStatus{NVIUnknown: true}
	}
	ctx, cancel := context.WithTimeout(ctx, nviCallTimeout)
	defer cancel()

	categories, err := c.nviCategories(ctx, bsn)
	if err != nil {
		return shareStatus{NVIUnknown: true}
	}
	return shareStatus{Shared: len(categories) > 0, Categories: categories}
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

	ctx, cancel := context.WithTimeout(ctx, nviCallTimeout)
	defer cancel()
	return c.nviRegister(ctx, patient.BSN, patient.PlataanCategories())
}
