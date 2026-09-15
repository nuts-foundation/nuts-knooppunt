package mitzmock

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// ClosedQuestionService is a mock MITZ server that handles XACML authorization requests
type ClosedQuestionService struct {
	server           *http.Server
	url              string
	requests         [][]byte // Store raw XML requests
	mu               sync.Mutex
	ResponseDecision string // "Permit" or "Deny"
	ResponseMessage  string
}

// New creates and starts a mock XACML MITZ server on listenAddr, which is
// anything net.Listen accepts ("localhost:0" for an ephemeral test port, ":8080"
// to serve the compose network). Callers own the returned service and must call
// Stop when done.
//
// This is the testing.T-free constructor, so the mock can run both in-process
// from tests (via NewClosedQuestionService) and as a standalone container in
// docker compose (via cmd/main.go), where a real Mitz is not available.
func New(listenAddr string) (*ClosedQuestionService, error) {
	mitz := &ClosedQuestionService{
		requests:         [][]byte{},
		ResponseDecision: "Permit", // Default to Permit
		ResponseMessage:  "Consent granted",
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
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
			slog.Error("mock XACML MITZ server error", "error", err)
		}
	}()

	slog.Info("Mock XACML MITZ server started", "url", mitz.url)

	return mitz, nil
}

// NewClosedQuestionService creates and starts a new mock XACML MITZ server on an
// ephemeral port, registering cleanup with t.
func NewClosedQuestionService(t *testing.T) *ClosedQuestionService {
	t.Helper()

	mitz, err := New("localhost:0")
	require.NoError(t, err, "failed to start mock XACML MITZ server")

	t.Cleanup(func() {
		if err := mitz.Stop(); err != nil {
			t.Logf("error stopping mock XACML MITZ server: %v", err)
		}
	})

	return mitz
}

// handleXACMLAuthz handles XACML authorization decision requests
func (m *ClosedQuestionService) handleXACMLAuthz(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Store raw XML request for verification
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, body)
	var decision = m.ResponseDecision
	if strings.Contains(string(body), "bsn:deny") {
		decision = "Deny"
	} else if strings.Contains(string(body), "bsn:error") {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	message := m.ResponseMessage

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
	_, _ = w.Write([]byte(responseXML))
}

// handleStatus handles status checks
func (m *ClosedQuestionService) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// GetURL returns the URL of the mock XACML MITZ server
func (m *ClosedQuestionService) GetURL() string {
	return m.url
}

// GetRequests returns all captured XACML authorization requests (raw XML)
func (m *ClosedQuestionService) GetRequests() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

// RequestCount returns the number of requests captured
func (m *ClosedQuestionService) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// SetResponse configures the response decision for subsequent requests
func (m *ClosedQuestionService) SetResponse(decision string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResponseDecision = decision
	m.ResponseMessage = message
}

// GetLastRequest returns the most recent XACML request (raw XML)
func (m *ClosedQuestionService) GetLastRequest() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return nil
	}
	return m.requests[len(m.requests)-1]
}

// GetLastRequestXML returns the XML representation of the most recent request for debugging
func (m *ClosedQuestionService) GetLastRequestXML() string {
	req := m.GetLastRequest()
	if req == nil {
		return ""
	}
	return string(req)
}

// Stop stops the mock XACML MITZ server
func (m *ClosedQuestionService) Stop() error {
	return m.server.Close()
}
