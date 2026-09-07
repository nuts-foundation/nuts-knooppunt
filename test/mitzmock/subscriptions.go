package mitzmock

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// SubscriptionService is a mock MITZ server that captures subscription requests
type SubscriptionService struct {
	server           *http.Server
	url              string
	subscriptions    *subscriptionStore
	ResponseStatus   int
	ResponseLocation string
}

// NewSubscriptionService creates and starts a new mock MITZ server
func NewSubscriptionService(t *testing.T) *SubscriptionService {
	t.Helper()

	mitz := &SubscriptionService{
		subscriptions:    newSubscriptionStore(),
		ResponseStatus:   http.StatusAccepted, // MITZ responds with 202 Accepted
		ResponseLocation: "Subscription/test-id-123",
	}

	// Find an available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}

	mitz.url = fmt.Sprintf("http://%s", listener.Addr().String())

	// Create HTTP server with mock handlers
	mux := http.NewServeMux()
	mux.HandleFunc("POST /abonnementen/fhir/Subscription", mitz.handleSubscription)
	mux.HandleFunc("GET /status", mitz.handleStatus)

	mitz.server = &http.Server{
		Handler: mux,
	}

	// Start server in background
	go func() {
		if err := mitz.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("mock MITZ server error: %v", err)
		}
	}()

	t.Logf("Mock MITZ server started at %s", mitz.url)
	t.Cleanup(func() {
		mitz.Stop(t)
	})

	return mitz
}

// handleSubscription handles subscription creation requests
func (m *SubscriptionService) handleSubscription(w http.ResponseWriter, r *http.Request) {
	var subscription fhir.Subscription
	if err := json.NewDecoder(r.Body).Decode(&subscription); err != nil {
		http.Error(w, fmt.Sprintf("failed to decode subscription: %v", err), http.StatusBadRequest)
		return
	}

	m.subscriptions.upsert(subscription)

	// Set response headers
	w.Header().Set("Location", m.ResponseLocation)
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(m.ResponseStatus)

	// Write response body if status is 200/201
	if m.ResponseStatus == http.StatusOK || m.ResponseStatus == http.StatusCreated {
		json.NewEncoder(w).Encode(subscription)
	}
}

// handleStatus handles status checks
func (m *SubscriptionService) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// GetURL returns the URL of the mock MITZ server
func (m *SubscriptionService) GetURL() string {
	return m.url
}

// GetSubscriptions returns all captured subscriptions
func (m *SubscriptionService) GetSubscriptions() []fhir.Subscription {
	return m.subscriptions.all()
}

// SubscriptionCount returns the number of subscriptions captured
func (m *SubscriptionService) SubscriptionCount() int {
	return len(m.subscriptions.all())
}

// Stop stops the mock MITZ server
func (m *SubscriptionService) Stop(t *testing.T) {
	t.Helper()
	if err := m.server.Close(); err != nil {
		t.Logf("error stopping mock MITZ server: %v", err)
	}
}

// subscriptionStore holds captured Mitz consent subscriptions, keyed by the
// (providerid, patientid) pair from the criteria string.
//
// Keyed rather than appended so that a demo which shares the same patient twice
// leaves one subscription, and so the acceptance criterion ("the mock contains a
// subscription for De Plataan and the selected patient", singular) is checkable.
// This is a property of the mock, chosen for deterministic demos. Revision 2 of
// the design is explicit that the same guarantee does not hold against the
// national Mitz.
type subscriptionStore struct {
	mu       sync.Mutex
	byKey    map[string]fhir.Subscription
	unkeyed  []fhir.Subscription
	sequence int
}

func newSubscriptionStore() *subscriptionStore {
	return &subscriptionStore{byKey: map[string]fhir.Subscription{}}
}

// subscriptionKey returns "providerid|patientid", or "" when either is absent.
// Both are required: keying on half a pair would merge unrelated subscriptions,
// and this mock does not run the component's parameter validation.
func subscriptionKey(s fhir.Subscription) string {
	params, err := url.ParseQuery(strings.TrimPrefix(s.Criteria, "Consent?"))
	if err != nil {
		return ""
	}
	provider, patient := params.Get("providerid"), params.Get("patientid")
	if provider == "" || patient == "" {
		return ""
	}
	return provider + "|" + patient
}

// upsert stores a subscription, returning it with its id and whether it was
// newly created.
func (s *subscriptionStore) upsert(sub fhir.Subscription) (fhir.Subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := subscriptionKey(sub)
	s.sequence++
	if key == "" {
		// Not keyable, so it cannot be deduplicated without merging unrelated
		// subscriptions. Keep it, separately.
		sub.Id = to.Ptr(fmt.Sprintf("mitzmock-unkeyed-%d", s.sequence))
		s.unkeyed = append(s.unkeyed, sub)
		return sub, true
	}
	if existing, ok := s.byKey[key]; ok {
		return existing, false
	}
	sub.Id = to.Ptr(fmt.Sprintf("mitzmock-%d", s.sequence))
	s.byKey[key] = sub
	return sub, true
}

// find returns the subscription for a provider/patient pair, if any. The sandbox
// uses it to reconcile after a response it did not receive.
func (s *subscriptionStore) find(providerID, patientID string) (fhir.Subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.byKey[providerID+"|"+patientID]
	return sub, ok
}

func (s *subscriptionStore) all() []fhir.Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]fhir.Subscription, 0, len(s.byKey)+len(s.unkeyed))
	for _, sub := range s.byKey {
		out = append(out, sub)
	}
	out = append(out, s.unkeyed...)
	// Numeric order, not lexicographic: "mitzmock-10" sorts before "mitzmock-2"
	// as a string, and the e2e reads subscriptions by index.
	sort.Slice(out, func(i, j int) bool { return idSequence(*out[i].Id) < idSequence(*out[j].Id) })
	return out
}

func (s *subscriptionStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.byKey)
	s.unkeyed = nil
}

// idSequence extracts the trailing counter from a generated id, so ordering is
// numeric. Unparseable ids sort last rather than panicking.
func idSequence(id string) int {
	idx := strings.LastIndex(id, "-")
	if idx < 0 {
		return math.MaxInt
	}
	n, err := strconv.Atoi(id[idx+1:])
	if err != nil {
		return math.MaxInt
	}
	return n
}

// handleCreateSubscription stores a consent subscription and answers 201 with a
// Location header, which is where component/mitz reads the id from.
func (m *ClosedQuestionService) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var sub fhir.Subscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, fmt.Sprintf("failed to decode subscription: %v", err), http.StatusBadRequest)
		return
	}
	stored, _ := m.subscriptions.upsert(sub)
	w.Header().Set("Location", "Subscription/"+*stored.Id)
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(stored)
}

// handleSearchSubscriptions returns everything captured, as a searchset Bundle,
// optionally filtered by the criteria's providerid/patientid. It exists so a test
// and the sandbox can establish what Mitz actually holds instead of inferring it
// from a status code that may never have arrived.
func (m *ClosedQuestionService) handleSearchSubscriptions(w http.ResponseWriter, r *http.Request) {
	var subs []fhir.Subscription
	provider, patient := r.URL.Query().Get("providerid"), r.URL.Query().Get("patientid")
	if provider != "" && patient != "" {
		if found, ok := m.subscriptions.find(provider, patient); ok {
			subs = []fhir.Subscription{found}
		}
	} else {
		subs = m.subscriptions.all()
	}

	entries := make([]fhir.BundleEntry, 0, len(subs))
	for _, sub := range subs {
		raw, err := json.Marshal(sub)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, fhir.BundleEntry{Resource: raw})
	}
	w.Header().Set("Content-Type", "application/fhir+json")
	_ = json.NewEncoder(w).Encode(fhir.Bundle{
		Type: fhir.BundleTypeSearchset, Total: to.Ptr(len(subs)), Entry: entries,
	})
}

// handleClearSubscriptions drops everything, so a demo reset restores a clean
// consent state (DESIGN §5.6).
func (m *ClosedQuestionService) handleClearSubscriptions(w http.ResponseWriter, _ *http.Request) {
	m.subscriptions.clear()
	w.WriteHeader(http.StatusNoContent)
}
