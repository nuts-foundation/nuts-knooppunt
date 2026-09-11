package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// patientRow is one row of the practitioner's patient list, and the header of
// the record and share screens.
type patientRow struct {
	Key                 string
	Name                string
	BSN                 string
	BirthDate           string
	Initials            string
	Shared              bool
	NVIUnknown          bool
	Subscribed          bool
	SubscriptionUnknown bool
	Locked              bool // held by another session: "demo in progress"
	Mine                bool // held by this session
	Categories          []string

	// UnnamedRecords counts NVI records this build cannot name. The share screen
	// needs it separately because it replaces Categories with what it proposes to
	// publish, which would otherwise present a stranger's record as one of the
	// set this client is about to write.
	UnnamedRecords int
}

// handlePatientList renders the practitioner's patients with their localization
// status. The NVI is queried once per patient, concurrently and each call with
// its own deadline: in sequence, one slow round trip would be multiplied by the
// pool on the first screen after login, and an unreachable NVI costs one unknown
// chip rather than the page.
func (c Config) handlePatientList(w http.ResponseWriter, r *http.Request, session *authSession) {
	patients := pool.Patients()
	statuses := make([]shareStatus, len(patients))

	var wg sync.WaitGroup
	for i, patient := range patients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = c.patientShareStatus(r.Context(), patient.BSN)
		}()
	}
	wg.Wait()

	rows := make([]patientRow, len(patients))
	for i, patient := range patients {
		rows[i] = c.rowFor(patient, statuses[i], session)
	}

	view := session.view()
	render(w, "ehr-patients.html", page{
		Title: "Patients · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Patients", ViewerOpen: true,
		Session: &view, Patients: rows, Notice: demoNotice(r),
	})
}

// patientDisplayName renders "Family, Given", the order a clinical list uses.
func patientDisplayName(p pool.PoolPatient) string {
	family := ""
	if p.Name.Family != nil {
		family = *p.Name.Family
	}
	given := ""
	if len(p.Name.Given) > 0 {
		given = p.Name.Given[0]
	}
	return family + ", " + given
}

func patientInitials(p pool.PoolPatient) string {
	initials := ""
	if len(p.Name.Given) > 0 {
		initials += firstLetter(p.Name.Given[0])
	}
	if p.Name.Family != nil {
		initials += firstLetter(*p.Name.Family)
	}
	return initials
}

// handleOpenPatient claims the patient for this session and redirects to the
// record. A POST, not a GET on the record itself: taking the lock changes shared
// state, denying other sessions and releasing this session's previous patient,
// which RFC 9110 §9.2.1 keeps off safe methods. The row submits it on one click.
func (c Config) handleOpenPatient(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site patient open is not allowed", http.StatusForbidden)
		return
	}
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	if !c.Locks.Switch(key, lockOwner(session)) {
		http.Redirect(w, r, "/demo/ehr?notice=patient-busy", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/demo/ehr/patients/"+key, http.StatusSeeOther)
}

// handlePatientRecord renders one patient's local record and its localization
// status. Read-only: the lock was taken by handleOpenPatient.
func (c Config) handlePatientRecord(w http.ResponseWriter, r *http.Request, session *authSession) {
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	row := c.rowFor(patient, c.patientShareStatus(r.Context(), patient.BSN), session)
	view := session.view()
	render(w, "ehr-record.html", page{
		Title: row.Name + " · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Patient record", ViewerOpen: true,
		Session: &view, Patient: &row, Notice: demoNotice(r),
	})
}

// rowFor builds the row the list, record and share screens render for one
// patient.
func (c Config) rowFor(patient pool.PoolPatient, status shareStatus, session *authSession) patientRow {
	mine := c.Locks.HeldBy(patient.Key, lockOwner(session))
	return patientRow{
		Key: patient.Key, Name: patientDisplayName(patient), BSN: patient.BSN,
		BirthDate: patient.BirthDate, Initials: patientInitials(patient),
		Locked: c.Locks.IsLocked(patient.Key) && !mine, Mine: mine,
		Shared: status.Shared, NVIUnknown: status.NVIUnknown, Categories: status.Categories,
		UnnamedRecords: status.UnnamedRecords,
		Subscribed:     status.Subscribed, SubscriptionUnknown: status.SubscriptionUnknown,
	}
}

// handleSubscribe retries only the Mitz step. This is what makes "no rollback"
// survivable: localization records without a consent subscription is a state the
// share flow can land in, and repairing it by re-running the whole share would
// redo the NVI work for nothing.
func (c Config) handleSubscribe(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site subscribe is not allowed", http.StatusForbidden)
		return
	}
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	if !c.Locks.HeldBy(patient.Key, lockOwner(session)) {
		http.Error(w, "this session does not hold patient "+patient.Key+"; reopen the record", http.StatusConflict)
		return
	}
	if c.mitzSubscribe == nil {
		http.Redirect(w, r, "/demo/ehr/patients/"+patient.Key+"?notice=mitz-disabled", http.StatusSeeOther)
		return
	}
	// The same critical section as the share flow, for the reason handleShare
	// gives.
	unlock := lockPatientWrites(patient.Key)
	defer unlock()
	attempt := c.subscribe(r.Context(), patient.BSN)
	http.Redirect(w, r, "/demo/ehr/patients/"+patient.Key+"?notice="+attempt.outcome.notice(), http.StatusSeeOther)
}

// mitzOutcome is what a subscription attempt established. The share flow reports
// it as a card and the retry route as a notice, and the distinctions are the
// point: a request that found a subscription did not start one, and a call that
// failed without a lookup to reconcile it did not fail.
type mitzOutcome int

const (
	// mitzStarted: absent before the call and present after it, so this request
	// created the subscription.
	mitzStarted mitzOutcome = iota
	// mitzRegistered: present after the call, but whether it existed before could
	// not be established, so this request may have found rather than created it.
	mitzRegistered
	// mitzExisting: present before and after. The call created nothing, whether
	// it succeeded or failed.
	mitzExisting
	// mitzFailed: the call failed and the lookup confirms there is no
	// subscription, so nothing was created and a retry cannot duplicate.
	mitzFailed
	// mitzUnknown: the call failed and the lookup could not be asked.
	mitzUnknown
)

// notice is the record-page notice the retry route redirects to.
func (o mitzOutcome) notice() string {
	switch o {
	case mitzStarted:
		return "mitz-retry-done"
	case mitzRegistered:
		return "mitz-retry-registered"
	case mitzExisting:
		return "mitz-retry-existing"
	case mitzFailed:
		return "mitz-retry-failed"
	}
	return "mitz-retry-unknown"
}

// mitzAttempt is one subscription attempt. callErr is set when the call failed;
// lookupErr says why the reconciling lookup could not be asked, for mitzUnknown.
type mitzAttempt struct {
	outcome   mitzOutcome
	callErr   error
	lookupErr error
}

// subscribe starts a consent subscription and establishes what happened. The
// caller holds the patient write lock: the gap between reading "no subscription"
// and creating one is where two requests can both conclude that they started it.
//
// The lookup is asked before the call so the outcome can say whether this request
// created the subscription or found one already there. The mock keys one
// subscription per provider and patient and answers a repeat with the existing
// one, so without the preflight a second share reports that it started something
// it did not.
//
// It is asked again after a failed call, because a timeout or dropped connection
// can follow a commit. Reporting a committed subscription as failed invites a
// retry that duplicates it against a Mitz which, unlike the mock, does not
// deduplicate. With no lookup at all the outcome is unknown, not failed.
func (c Config) subscribe(ctx context.Context, bsn string) mitzAttempt {
	existedBefore, preflightErr := c.subscriptionExists(ctx, bsn)
	knewBefore := preflightErr == nil

	if err := c.mitzSubscribe(ctx, bsn); err != nil {
		subscribed, lookupErr := c.subscriptionExists(ctx, bsn)
		switch {
		case lookupErr != nil:
			return mitzAttempt{outcome: mitzUnknown, callErr: err, lookupErr: lookupErr}
		case !subscribed:
			return mitzAttempt{outcome: mitzFailed, callErr: err}
		}
		// It committed after all; only its response went missing.
	}
	switch {
	case !knewBefore:
		return mitzAttempt{outcome: mitzRegistered}
	case existedBefore:
		return mitzAttempt{outcome: mitzExisting}
	}
	return mitzAttempt{outcome: mitzStarted}
}

// subscriptionExists answers the reconciliation question, distinguishing "no
// subscription" from "could not ask". A nil lookup is the second: MITZMOCK_URL is
// unset, and reading silence as a definite negative is what would put a red
// failure card over a subscription that exists.
func (c Config) subscriptionExists(ctx context.Context, bsn string) (bool, error) {
	if c.mitzSubscribed == nil {
		return false, errors.New("Mitz cannot be queried in this environment")
	}
	return c.mitzSubscribed(ctx, bsn)
}

// cardResult is one confirmation card. Skipped and Unknown are states beside OK
// and Failed because the next safe action differs for each: "not attempted" is
// not "failed", and "we could not establish what happened" is not "it did not
// happen". A red card with a cross asserts a failure, so an unknown outcome
// rendered that way is the demo claiming knowledge it does not have.
type cardResult struct {
	Title   string
	Detail  string
	OK      bool
	Failed  bool
	Skipped bool
	Unknown bool
	Rows    []cardRow
}

type cardRow struct{ Key, Value string }

// handleShareForm renders the share screen. Read-only; the lock was taken when
// the patient was opened.
func (c Config) handleShareForm(w http.ResponseWriter, r *http.Request, session *authSession) {
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	c.renderShare(w, r, session, patient, nil, nil)
}

// handleShare publishes the localization records and starts the Mitz
// subscription, then renders both outcomes.
//
// Ownership is checked here, not trusted from the page: a tab whose lease expired,
// or one left open on another patient, still renders an actionable button. It is
// checked once, at the start; the README records that the lease is not held for
// the duration.
func (c Config) handleShare(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site share is not allowed", http.StatusForbidden)
		return
	}
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	if !c.Locks.HeldBy(patient.Key, lockOwner(session)) {
		http.Error(w, "this session does not hold patient "+patient.Key+"; reopen the record", http.StatusConflict)
		return
	}

	// One critical section over both remote steps, not one per step. The Mitz
	// half asks whether a subscription exists before creating one, and a lock
	// that ends between the two lets a double submit run both preflights while
	// the answer is still no; both would then claim to have started the one
	// subscription the mock deduplicated. Per process, like the NVI
	// serialization it replaces; two sandbox processes are still unordered.
	unlock := lockPatientWrites(patient.Key)
	defer unlock()

	nviCard := c.shareNVI(r.Context(), patient)
	mitzCard := c.shareMitz(r.Context(), patient, nviCard.Failed)
	c.renderShare(w, r, session, patient, nviCard, mitzCard)
}

func (c Config) shareNVI(ctx context.Context, patient pool.PoolPatient) *cardResult {
	if err := c.registerPatient(ctx, patient); err != nil {
		// One card for every NVI failure. Register is search-delete-create behind
		// an opaque error, so a failed search, a failed create and a committed
		// write with a lost response are indistinguishable here; the card states
		// the ambiguity and the action rather than inventing which it was.
		return &cardResult{
			Failed: true,
			Title:  "Localization records may be incomplete",
			Detail: "The NVI did not confirm the update: " + err.Error() + ". Some records may have " +
				"been removed or created. Sharing again attempts to reconcile; it cannot guarantee " +
				"it, and a configuration problem will keep failing until it is fixed.",
		}
	}
	return &cardResult{
		OK:    true,
		Title: "Localization records published",
		Detail: "Other care providers can now find that De Plataan holds data for this patient. " +
			"Only the pointer was published; no clinical data left De Plataan.",
		Rows: []cardRow{
			{Key: "Holder", Value: "Ziekenhuis De Plataan · URA " + plataanURA},
			{Key: "Data categories", Value: strings.Join(patient.PlataanCategories(), ", ")},
			{Key: "Patient", Value: "as a pseudonym, via the pseudonym service"},
		},
	}
}

func (c Config) shareMitz(ctx context.Context, patient pool.PoolPatient, nviFailed bool) *cardResult {
	if nviFailed {
		// Nothing to subscribe consent changes about, and the retry redoes the
		// NVI step anyway.
		return &cardResult{Skipped: true, Title: "Mitz subscription",
			Detail: "Not attempted: the localization step did not succeed."}
	}
	if c.mitzSubscribe == nil {
		return &cardResult{Skipped: true, Title: "Mitz subscription",
			Detail: "Not attempted: Mitz is not wired up in this environment."}
	}

	attempt := c.subscribe(ctx, patient.BSN)
	title, detail := "Consent subscription started", "This request registered the subscription at Mitz."
	switch attempt.outcome {
	case mitzUnknown:
		return &cardResult{
			Unknown: true,
			Title:   "Mitz subscription outcome unknown",
			Detail: "The call did not complete: " + attempt.callErr.Error() + ". Mitz could not be asked whether " +
				"it went through anyway (" + attempt.lookupErr.Error() + "), so it is not known whether a subscription exists. " +
				"The record page keeps this visible and offers a retry of this step alone; against the " +
				"national Mitz that retry may create a second subscription.",
		}
	case mitzFailed:
		return &cardResult{
			Failed: true,
			Title:  "Mitz subscription failed",
			Detail: "The patient is shared, but no consent subscription exists: " + attempt.callErr.Error() +
				". Mitz confirms there is none, so nothing was created and a retry cannot " +
				"duplicate. The record page keeps this visible and offers a retry of this step alone.",
		}
	case mitzRegistered:
		title = "Consent subscription registered"
		detail = "Mitz accepted the subscription. Whether one already existed could not be " +
			"established here, so this may have found rather than created it."
	case mitzExisting:
		// Registered, not active: the sandbox sends status "requested", the mock
		// stores it unchanged, and the lookup only asks whether a Subscription
		// exists, which says nothing about the FHIR lifecycle.
		title = "Consent subscription already registered"
		detail = "A subscription for this patient at De Plataan was already registered at Mitz, so " +
			"this request did not create a second one."
	}
	return &cardResult{
		OK:    true,
		Title: title,
		// What the subscription is, not what will arrive: the sandbox configures
		// no notification endpoint and nothing listens for one, so promising
		// delivery would assert a message the stack cannot send.
		Detail: detail + " Mitz holds it against this patient's consent; delivering the notifications " +
			"themselves is not wired up in the sandbox, so nothing arrives here when consent changes.",
		Rows: []cardRow{
			{Key: "Subscriber", Value: "Ziekenhuis De Plataan"},
			{Key: "Registry", Value: "Mitz (simulated)"},
		},
	}
}

func (c Config) renderShare(w http.ResponseWriter, r *http.Request, session *authSession,
	patient pool.PoolPatient, nviCard, mitzCard *cardResult) {
	row := c.rowFor(patient, c.patientShareStatus(r.Context(), patient.BSN), session)
	// The share screen names what would be, or has just been, registered, not
	// what the NVI holds. UnnamedRecords survives the swap and is what the
	// template uses to say the two are not the same set.
	row.Categories = patient.PlataanCategories()

	// A successful publish is knowledge the follow-up read cannot take away. That
	// read can fail on its own, and letting it decide the header would put "this
	// record exists only in De Plataan's own store" under the green card that
	// just reported the publish.
	if nviCard != nil && nviCard.OK {
		row.Shared, row.NVIUnknown = true, false
	}
	// Subscribed and SubscriptionUnknown are left as read, because this template
	// renders no subscription chip. One added here must be overridden from
	// mitzCard the same way.

	view := session.view()
	render(w, "ehr-share.html", page{
		Title: "Share patient · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Patient registration", ViewerOpen: true,
		Session: &view, Patient: &row, NVICard: nviCard, MitzCard: mitzCard,
	})
}
