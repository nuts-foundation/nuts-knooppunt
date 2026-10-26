package main

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// newChangeID returns a random version 4 UUID. The mock uses it as the id of
// the notification Bundle, which is how a notification is paired with the
// action that caused it.
func newChangeID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// handleConsent is the demo's stand-in for the patient acting in Mitz: it
// changes the mock's answer for this patient. Everything that follows from
// that, the notification and the refusal of a new retrieval, comes back through
// the real paths.
func (c Config) handleConsent(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site consent change is not allowed", http.StatusForbidden)
		return
	}
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "unreadable form", http.StatusBadRequest)
		return
	}
	want, mockDecision := r.FormValue("decision"), ""
	switch want {
	case "deny":
		mockDecision = "Deny"
	case "permit":
		mockDecision = "Permit"
	default:
		http.Error(w, `decision must be "deny" or "permit"`, http.StatusBadRequest)
		return
	}
	owner := lockOwner(session)
	if !c.Locks.HeldBy(patient.Key, owner) {
		http.Error(w, "this session does not hold patient "+patient.Key+"; reopen the record", http.StatusConflict)
		return
	}
	record := "/demo/ehr/patients/" + patient.Key
	if c.mitzSetConsent == nil || c.mitzConsentState == nil {
		http.Redirect(w, r, record+"?notice=mitz-disabled", http.StatusSeeOther)
		return
	}
	runID := c.Runs.current(owner, patient.Key)

	// The patient's mutex first, then the gate: requests queued for this patient
	// wait here without holding the gate, so a reset is delayed only by the one
	// action that is running. Neither is taken while a store lock is held, and
	// the intake takes neither, which is what lets the notification arrive
	// while this request waits on the mock.
	lock := c.runtime.consentLock(patient.Key)
	c.runtime.at(probeConsentQueued, patient.Key) // admitted once; nothing is held yet
	lock.Lock()
	defer lock.Unlock()
	c.runtime.at(probeConsentTurn, patient.Key) // its turn: the patient's mutex, and not the gate yet
	c.runtime.gate.RLock()
	defer c.runtime.gate.RUnlock()

	// Admission again, now that the waiting is over. A reset may have replaced
	// the world this request was admitted to, or the session may have moved to
	// another patient; acting on the earlier check would change consent on
	// behalf of a run that no longer exists.
	if r.Context().Err() != nil || !c.Locks.HeldBy(patient.Key, owner) || runID == "" ||
		c.Runs.current(owner, patient.Key) != runID {
		http.Redirect(w, r, "/demo/ehr?notice=patient-not-open", http.StatusSeeOther)
		return
	}
	if c.sessions != nil && !c.sessions.storeIfLive(session.ID, func() {}) {
		http.Redirect(w, r, "/demo/login", http.StatusSeeOther)
		return
	}

	action, _ := r.Context().Value(eventActionKey{}).(eventAction)
	capture, _ := r.Context().Value(eventCaptureKey{}).(eventCapture)
	changeID := newChangeID()
	c.runtime.openPending(patient.Key, pendingConsent{actionID: action.id, runID: runID, changeID: changeID})
	receivedBefore := c.runtime.receivedCount(patient.Key)

	changedAt, callErr := c.mitzSetConsent(r.Context(), patient.BSN, mockDecision, changeID)

	// The capture transport has published the change's event by now, also when
	// the call failed. The notifications handed to this action follow it, and
	// the entry is closed in the same step, so one arriving later is published
	// on its own.
	for _, event := range c.runtime.closePending(patient.Key, changeID) {
		event.ActionID, event.Action = action.id, action.kind
		if capture.emit != nil {
			capture.emit(event)
		}
	}
	// Whatever came of it, the card on other open pages of this record may now
	// be wrong.
	defer c.runtime.advance(patient.Key)

	if callErr != nil {
		// The answer was lost, not necessarily the change. Ask what Mitz says
		// now.
		state, stateErr := c.mitzConsentState(r.Context(), patient.BSN)
		if stateErr != nil {
			http.Redirect(w, r, record+"?notice=consent-unknown", http.StatusSeeOther)
			return
		}
		// Before the answer is compared: whatever it says, its timestamp is one
		// Mitz issued for this patient, and a reset that cannot reach the mock
		// falls back on the latest the sandbox has seen.
		if !state.ChangedAt.IsZero() {
			c.runtime.noteIssued(patient.Key, state.ChangedAt)
		}
		if state.Decision != mockDecision {
			http.Redirect(w, r, record+"?notice=consent-failed", http.StatusSeeOther)
			return
		}
		changedAt = state.ChangedAt
	}
	if !changedAt.IsZero() {
		c.runtime.noteIssued(patient.Key, changedAt)
	}

	// Notified is what this application itself received, not what the mock says
	// it delivered: the mock knows the Knooppunt answered, not that the
	// notification reached the EHR.
	latest, _ := c.runtime.notification(patient.Key)
	notified := c.runtime.receivedCount(patient.Key) > receivedBefore && latest.Decision == want
	notice := map[string]map[bool]string{
		"deny":   {true: "consent-withdrawn", false: "consent-withdrawn-unnotified"},
		"permit": {true: "consent-restored", false: "consent-restored-unnotified"},
	}[want][notified]
	http.Redirect(w, r, record+"?notice="+notice, http.StatusSeeOther)
}
