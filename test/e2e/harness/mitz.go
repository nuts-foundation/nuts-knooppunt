package harness

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// MockMITZServer is a mock MITZ server that captures subscription requests
type MockMITZServer struct {
	server           *http.Server
	url              string
	subscriptions    []fhir.Subscription
	mu               sync.Mutex
	ResponseStatus   int
	ResponseLocation string
}

// NewMockMITZServer creates and starts a new mock MITZ server
func NewMockMITZServer(t *testing.T) *MockMITZServer {
	t.Helper()

	mitz := &MockMITZServer{
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
	mux.HandleFunc("POST /geslotenautorisatievraag/xacml3", mitz.handleGeslotenVraag)
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
func (m *MockMITZServer) handleSubscription(w http.ResponseWriter, r *http.Request) {
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
func (m *MockMITZServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (m *MockMITZServer) handleGeslotenVraag(httpResponse http.ResponseWriter, _ *http.Request) {
	// Return a hardcoded XACML Permit decision in SOAP envelope format
	xacmlResponse := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>Permit</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`

	httpResponse.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	httpResponse.WriteHeader(http.StatusOK)
	_, _ = httpResponse.Write([]byte(xacmlResponse))
}

// GetURL returns the URL of the mock MITZ server
func (m *MockMITZServer) GetURL() string {
	return m.url
}

// GetSubscriptions returns all captured subscriptions
func (m *MockMITZServer) GetSubscriptions() []fhir.Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subscriptions
}

// SubscriptionCount returns the number of subscriptions captured
func (m *MockMITZServer) SubscriptionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.subscriptions)
}

// Stop stops the mock MITZ server
func (m *MockMITZServer) Stop(t *testing.T) {
	t.Helper()
	if err := m.server.Close(); err != nil {
		t.Logf("error stopping mock MITZ server: %v", err)
	}
}
