package mitzmock

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// consentOverride is the answer the mock gives about one patient, in place of
// its default, and when that answer was stored.
type consentOverride struct {
	Decision  string
	ChangedAt time.Time
}

// consentChange is one stored change, as delivery needs it.
type consentChange struct {
	BSN, Decision, ChangeID string
	ChangedAt               time.Time
}

// notificationResult is what one delivery attempt of a change came to.
type notificationResult struct {
	SubscriptionID string `json:"subscriptionId"`
	Endpoint       string `json:"endpoint"`
	Status         int    `json:"status"`
	Error          string `json:"error"`
}

// consentState holds the per-patient overrides and the locks that order their
// changes.
//
// A change holds its patient's mutex from storing the override to the end of its
// delivery, so two changes for one patient deliver in the order they were
// stored. It also holds changes for reading; the delete of everything holds it
// for writing, and so waits for every running change. Neither is the mutex that
// guards the map, which is never held across network I/O.
type consentState struct {
	mu        sync.Mutex
	overrides map[string]consentOverride
	locks     map[string]*sync.Mutex
	changes   sync.RWMutex
}

func newConsentState() *consentState {
	return &consentState{overrides: map[string]consentOverride{}, locks: map[string]*sync.Mutex{}}
}

func (s *consentState) lockFor(bsn string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[bsn] == nil {
		s.locks[bsn] = &sync.Mutex{}
	}
	return s.locks[bsn]
}

func (s *consentState) get(bsn string) (consentOverride, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	override, ok := s.overrides[bsn]
	return override, ok
}

func (s *consentState) set(bsn string, override consentOverride) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrides[bsn] = override
}

func (s *consentState) remove(bsn string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overrides, bsn)
}

func (s *consentState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.overrides)
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// newUUID returns a random version 4 UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func writeMockJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// handlePutConsent changes the answer for one patient and notifies its
// subscribers. This is the simulated patient act; the national Mitz registers
// consent through another interface altogether.
func (m *ClosedQuestionService) handlePutConsent(w http.ResponseWriter, r *http.Request) {
	bsn := r.PathValue("bsn")
	var request struct {
		Decision string `json:"decision"`
		ChangeID string `json:"changeId"`
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil || json.Unmarshal(raw, &request) != nil {
		http.Error(w, "the body must be JSON with a decision", http.StatusBadRequest)
		return
	}
	if request.Decision != "Permit" && request.Decision != "Deny" {
		http.Error(w, `decision must be "Permit" or "Deny"`, http.StatusBadRequest)
		return
	}
	if request.ChangeID == "" {
		request.ChangeID = newUUID()
	} else if !uuidPattern.MatchString(request.ChangeID) {
		http.Error(w, "changeId must be a lowercase UUID", http.StatusBadRequest)
		return
	}

	// The patient's mutex first, then the shared one: a request queued behind
	// another change for the same patient must not hold up a cleanup.
	patient := m.consent.lockFor(bsn)
	patient.Lock()
	defer patient.Unlock()
	m.consent.changes.RLock()
	defer m.consent.changes.RUnlock()

	change := consentChange{BSN: bsn, Decision: request.Decision, ChangeID: request.ChangeID, ChangedAt: m.clock.next()}
	m.consent.set(bsn, consentOverride{Decision: change.Decision, ChangedAt: change.ChangedAt})

	results := m.deliverChange(change)
	writeMockJSON(w, map[string]any{
		"bsn": bsn, "decision": change.Decision, "changeId": change.ChangeID,
		"changedAt": change.ChangedAt.Format(time.RFC3339Nano), "notifications": results,
	})
}

func (m *ClosedQuestionService) handleGetConsent(w http.ResponseWriter, r *http.Request) {
	bsn := r.PathValue("bsn")
	m.mu.Lock()
	decision := m.ResponseDecision
	m.mu.Unlock()
	body := map[string]any{"bsn": bsn, "decision": decision, "overridden": false}
	if override, ok := m.consent.get(bsn); ok {
		body["decision"], body["overridden"] = override.Decision, true
		body["changedAt"] = override.ChangedAt.Format(time.RFC3339Nano)
	}
	writeMockJSON(w, body)
}

// handleDeleteConsent restores one patient to the default, silently: a reset is
// not a patient act. It waits for a change of that patient that is still being
// delivered, and its watermark is later than every change stored before it.
func (m *ClosedQuestionService) handleDeleteConsent(w http.ResponseWriter, r *http.Request) {
	bsn := r.PathValue("bsn")
	patient := m.consent.lockFor(bsn)
	patient.Lock()
	defer patient.Unlock()
	m.consent.remove(bsn)
	m.decisions.deletePatient(bsn)
	writeMockJSON(w, map[string]string{"watermark": m.clock.next().Format(time.RFC3339Nano)})
}

func (m *ClosedQuestionService) handleDeleteAllConsent(w http.ResponseWriter, _ *http.Request) {
	m.consent.changes.Lock()
	defer m.consent.changes.Unlock()
	m.consent.clear()
	m.decisions.clear()
	writeMockJSON(w, map[string]string{"watermark": m.clock.next().Format(time.RFC3339Nano)})
}
