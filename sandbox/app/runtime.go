package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// consentNotification is the latest thing Mitz told De Plataan about one
// patient's consent.
type consentNotification struct {
	Decision    string    // "deny" or "permit"
	ConsentTime time.Time // Consent.dateTime, which the mock issues in order
	ReceivedAt  time.Time
}

// pendingConsent is a consent action that is waiting on the mock. A
// notification whose Bundle id is its change id is handed to it, so the action
// can publish its own event first and the notification's after it.
type pendingConsent struct {
	actionID, runID, changeID string
	events                    []stepEvent
}

// The points at which runtimeState.at is called.
const (
	probeIntakeArrived    = "intake:arrived"    // a notification for a pool patient, before its transition
	probeAcceptCommitting = "accept:committing" // inside accept, past the watermark check, before the commit; the state mutex is held
	probeIntakeAccepted   = "intake:accepted"   // after its transition, before its event goes anywhere
	probePublishing       = "intake:publishing" // inside the publication fence, after the reset check
	probeConsentQueued    = "consent:queued"    // an action admitted once, before it takes any lock
	probeConsentTurn      = "consent:turn"      // it holds its patient's mutex, and not the gate yet
	probeResetApplied     = "reset:applied"     // a patient's reset transition is over
)

// runtimeState is what the main application and the notification intake share.
// Config is copied by value into every handler and main builds its listeners
// beside the mux, so anything both must see lives here, behind one pointer that
// every copy of Config carries. No mutex in it is ever copied.
//
// Four groups, each with its own lock, in this order when more than one is
// needed: the action locks, the pending entries, the publication fence, the
// state. The session, run and registry stores come between the fence and the
// state. The state mutex is taken before the retrieval store's, and nothing
// above it, and no network call, is waited for while it is held.
type runtimeState struct {
	// gate is held for reading by a consent action and for writing by reset and
	// recycle, which thereby wait for the actions already admitted.
	gate sync.RWMutex

	locksMu      sync.Mutex
	consentLocks map[string]*sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]*pendingConsent // by patient key

	// publication is held for reading around the check of a patient's reset
	// counter and the publication that depends on it, and for writing by a
	// reset's transition, so that no reset lands between the two.
	publication sync.RWMutex

	stateMu       sync.Mutex
	notifications map[string]consentNotification
	received      map[string]uint64
	// watermark is the latest mock-issued timestamp below which a notification
	// for the patient is ignored. It only rises, and is never cleared.
	watermark map[string]time.Time
	// issued is the latest changedAt the sandbox itself saw for the patient.
	issued map[string]time.Time
	// mockWatermarks are what the mock's deletes answered, by BSN; "" is the
	// delete of everything.
	mockWatermarks map[string]time.Time
	revision       map[string]uint64
	resets         map[string]uint64
	changed        map[string]chan struct{}

	// probe is called at the points named above. Nothing but a test sets it.
	probe atomic.Pointer[func(point, patientKey string)]
}

func newRuntimeState() *runtimeState {
	return &runtimeState{
		consentLocks: map[string]*sync.Mutex{}, pending: map[string]*pendingConsent{},
		notifications: map[string]consentNotification{}, received: map[string]uint64{},
		watermark: map[string]time.Time{}, issued: map[string]time.Time{}, mockWatermarks: map[string]time.Time{},
		revision: map[string]uint64{}, resets: map[string]uint64{}, changed: map[string]chan struct{}{},
	}
}

// at calls the test probe, if one is set. Outside tests it does nothing.
func (s *runtimeState) at(point, patientKey string) {
	if probe := s.probe.Load(); probe != nil {
		(*probe)(point, patientKey)
	}
}

// consentLock returns the mutex that makes consent actions for one patient run
// one at a time.
func (s *runtimeState) consentLock(patientKey string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	if s.consentLocks[patientKey] == nil {
		s.consentLocks[patientKey] = &sync.Mutex{}
	}
	return s.consentLocks[patientKey]
}

func (s *runtimeState) openPending(patientKey string, pending pendingConsent) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	s.pending[patientKey] = &pending
}

// handOff gives a notification's event to the action whose change caused it,
// and reports whether there was one. Only the change id pairs them: not the
// run, not the decision, not the moment.
func (s *runtimeState) handOff(patientKey, changeID string, event stepEvent) bool {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	pending := s.pending[patientKey]
	if pending == nil || changeID == "" || pending.changeID != changeID {
		return false
	}
	pending.events = append(pending.events, event)
	return true
}

// closePending removes the action's own entry and returns the events handed to
// it, in one step, so an event is either returned here or, arriving later,
// refused by handOff and published on its own. Exactly one of the two.
func (s *runtimeState) closePending(patientKey, changeID string) []stepEvent {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	pending := s.pending[patientKey]
	if pending == nil || pending.changeID != changeID {
		return nil
	}
	delete(s.pending, patientKey)
	return pending.events
}

// accept is the one transition by which a notification changes what the record
// shows. It refuses a notification that is not strictly later than the
// patient's watermark: a duplicate, one overtaken by a newer one, or one of a
// change made before a reset. apply runs inside the transition and is where the
// retrieval store is changed. It returns the patient's reset counter as it was
// at the transition, which guards the event's publication.
func (s *runtimeState) accept(patientKey string, n consentNotification, apply func()) (bool, uint64) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if !n.ConsentTime.After(s.watermark[patientKey]) {
		return false, s.resets[patientKey]
	}
	// The check above and the commit below are one step: the mutex is held from
	// the one to the other. A newer notification that got in between would be
	// committed first and then overwritten by this older one.
	s.at(probeAcceptCommitting, patientKey)
	s.watermark[patientKey] = n.ConsentTime
	s.notifications[patientKey] = n
	s.received[patientKey]++
	if apply != nil {
		apply()
	}
	s.advanceLocked(patientKey)
	return true, s.resets[patientKey]
}

func (s *runtimeState) notification(patientKey string) (consentNotification, bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	n, ok := s.notifications[patientKey]
	return n, ok
}

func (s *runtimeState) receivedCount(patientKey string) uint64 {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.received[patientKey]
}

// noteIssued remembers a changedAt the mock told the sandbox for this patient,
// in the answer to a change or to a state read. A reset that gets no watermark
// from the mock falls back on the latest of them.
func (s *runtimeState) noteIssued(patientKey string, changedAt time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if changedAt.After(s.issued[patientKey]) {
		s.issued[patientKey] = changedAt
	}
}

// noteMockWatermark remembers what a delete at the mock answered.
func (s *runtimeState) noteMockWatermark(bsn string, watermark time.Time) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if watermark.After(s.mockWatermarks[bsn]) {
		s.mockWatermarks[bsn] = watermark
	}
}

// reset is what a reset or a recycle does to one patient's state: the
// notification goes, the reset counter and the revision advance, and the
// watermark rises to the latest of what the mock answered and what the sandbox
// saw, so that a notification of a change made before the reset is ignored
// however late it is handled. clear runs inside the transition and is where the
// patient's retrievals and removal marks are dropped.
func (s *runtimeState) reset(patientKey, bsn string, clear func()) {
	func() {
		// The fence first: a publication that has checked this patient's counter
		// finishes before the counter moves.
		s.publication.Lock()
		defer s.publication.Unlock()
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		s.resets[patientKey]++
		delete(s.notifications, patientKey)
		for _, floor := range []time.Time{s.mockWatermarks[""], s.mockWatermarks[bsn], s.issued[patientKey]} {
			if floor.After(s.watermark[patientKey]) {
				s.watermark[patientKey] = floor
			}
		}
		if clear != nil {
			clear()
		}
		s.advanceLocked(patientKey)
	}()
	s.at(probeResetApplied, patientKey)
}

func (s *runtimeState) resetsOf(patientKey string) uint64 {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.resets[patientKey]
}

// publishUnlessReset runs publish unless the patient was reset since resets was
// read, and reports whether it ran. The fence is held around the check and the
// publication together. The state mutex is not: publish takes the session, run
// and registry locks, which come before it.
func (s *runtimeState) publishUnlessReset(patientKey string, resets uint64, publish func()) bool {
	s.publication.RLock()
	defer s.publication.RUnlock()
	if s.resetsOf(patientKey) != resets {
		return false
	}
	s.at(probePublishing, patientKey)
	publish()
	return true
}

// change is the transition for a change to what the patient's record shows that
// is not a notification: a retrieval stored, forgotten or discarded. apply runs
// inside it and is where the retrieval store is changed, so a snapshot sees the
// store and the revision either both before the change or both after it.
func (s *runtimeState) change(patientKey string, apply func()) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if apply != nil {
		apply()
	}
	s.advanceLocked(patientKey)
}

// advance marks a change to what the patient's record shows that happened
// outside the stores this state guards, such as a consent change at Mitz.
func (s *runtimeState) advance(patientKey string) { s.change(patientKey, nil) }

func (s *runtimeState) advanceLocked(patientKey string) {
	s.revision[patientKey]++
	if watchers := s.changed[patientKey]; watchers != nil {
		close(watchers)
	}
	s.changed[patientKey] = make(chan struct{})
}

// snapshot returns the patient's record revision and a channel that is closed
// at its next advance. read runs under the state mutex, which every transition
// holds, so what it reads from the retrieval store belongs to exactly that
// revision. Reading changes nothing.
func (s *runtimeState) snapshot(patientKey string, read func()) (uint64, <-chan struct{}) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.changed[patientKey] == nil {
		s.changed[patientKey] = make(chan struct{})
	}
	if read != nil {
		read()
	}
	return s.revision[patientKey], s.changed[patientKey]
}

// recordState is snapshot for a caller that reads nothing else.
func (s *runtimeState) recordState(patientKey string) (uint64, <-chan struct{}) {
	return s.snapshot(patientKey, nil)
}
