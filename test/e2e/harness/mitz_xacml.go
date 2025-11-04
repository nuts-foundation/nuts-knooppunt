package harness

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
)

// MockXACMLMitzServer is a mock MITZ server that handles XACML authorization requests
type MockXACMLMitzServer struct {
	server           *http.Server
	url              string
	requests         [][]byte // Store raw XML requests
	mu               sync.Mutex
	ResponseDecision string // "Permit" or "Deny"
	ResponseMessage  string
}

// NewMockXACMLMitzServer creates and starts a new mock XACML MITZ server
func NewMockXACMLMitzServer(t *testing.T) *MockXACMLMitzServer {
	t.Helper()

	mitz := &MockXACMLMitzServer{
		requests:         [][]byte{},
		ResponseDecision: "Permit", // Default to Permit
		ResponseMessage:  "Consent granted",
	}

	// Find an available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}

	mitz.url = fmt.Sprintf("http://%s", listener.Addr().String())

	// Create HTTP server with mock handlers
	mux := http.NewServeMux()
	mux.HandleFunc("POST /geslotenautorisatievraag/xacml3", mitz.handleXACMLAuthz)
	mux.HandleFunc("GET /status", mitz.handleStatus)

	mitz.server = &http.Server{
		Handler: mux,
	}

	// Start server in background
	go func() {
		if err := mitz.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("mock XACML MITZ server error: %v", err)
		}
	}()

	t.Logf("Mock XACML MITZ server started at %s", mitz.url)
	t.Cleanup(func() {
		mitz.Stop(t)
	})

	return mitz
}

// handleXACMLAuthz handles XACML authorization decision requests
func (m *MockXACMLMitzServer) handleXACMLAuthz(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Store raw XML request for verification
	m.mu.Lock()
	m.requests = append(m.requests, body)
	decision := m.ResponseDecision
	message := m.ResponseMessage
	m.mu.Unlock()

	// Build XACML SOAP response XML
	responseXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
      <Result>
        <Decision>%s</Decision>
        <Status>
          <StatusCode Value="urn:oasis:names:tc:xacml:1.0:status:ok"/>
          <StatusMessage>%s</StatusMessage>
        </Status>
      </Result>
    </Response>
  </s:Body>
</s:Envelope>`, decision, message)

	// Set response headers
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responseXML))
}

// handleStatus handles status checks
func (m *MockXACMLMitzServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// GetURL returns the URL of the mock XACML MITZ server
func (m *MockXACMLMitzServer) GetURL() string {
	return m.url
}

// GetRequests returns all captured XACML authorization requests (raw XML)
func (m *MockXACMLMitzServer) GetRequests() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

// RequestCount returns the number of requests captured
func (m *MockXACMLMitzServer) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// SetResponse configures the response decision for subsequent requests
func (m *MockXACMLMitzServer) SetResponse(decision string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResponseDecision = decision
	m.ResponseMessage = message
}

// GetLastRequest returns the most recent XACML request (raw XML)
func (m *MockXACMLMitzServer) GetLastRequest() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return nil
	}
	return m.requests[len(m.requests)-1]
}

// GetLastRequestXML returns the XML representation of the most recent request for debugging
func (m *MockXACMLMitzServer) GetLastRequestXML() string {
	req := m.GetLastRequest()
	if req == nil {
		return ""
	}
	return string(req)
}

// Stop stops the mock XACML MITZ server
func (m *MockXACMLMitzServer) Stop(t *testing.T) {
	t.Helper()
	if err := m.server.Close(); err != nil {
		t.Logf("error stopping mock XACML MITZ server: %v", err)
	}
}
