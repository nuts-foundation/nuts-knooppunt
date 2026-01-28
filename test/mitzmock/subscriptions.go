package mitzmock

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// SubscriptionService is a mock MITZ server that captures subscription requests
type SubscriptionService struct {
	server           *http.Server
	url              string
	subscriptions    []fhir.Subscription
	mu               sync.Mutex
	ResponseStatus   int
	ResponseLocation string
}

// NewSubscriptionService creates and starts a new mock MITZ server
func NewSubscriptionService(t *testing.T) *SubscriptionService {
	t.Helper()

	mitz := &SubscriptionService{
		subscriptions:    []fhir.Subscription{},
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

	m.mu.Lock()
	m.subscriptions = append(m.subscriptions, subscription)
	m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subscriptions
}

// SubscriptionCount returns the number of subscriptions captured
func (m *SubscriptionService) SubscriptionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.subscriptions)
}

// Stop stops the mock MITZ server
func (m *SubscriptionService) Stop(t *testing.T) {
	t.Helper()
	if err := m.server.Close(); err != nil {
		t.Logf("error stopping mock MITZ server: %v", err)
	}
}
