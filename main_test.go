package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems"
	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems/nuts"
)

func TestSubsystemIntegration(t *testing.T) {
	// Create subsystem manager
	manager := subsystems.NewManager()

	// Register Nuts node subsystem
	nutsSubsystem := nuts.NewSubsystem()
	if err := manager.Register(nutsSubsystem); err != nil {
		t.Fatalf("Failed to register Nuts subsystem: %v", err)
	}

	// Start all subsystems
	if err := manager.Start(nil); err != nil {
		t.Fatalf("Failed to start subsystems: %v", err)
	}

	// Give time for servers to start
	time.Sleep(200 * time.Millisecond)

	// Create main HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": "Nuts Knooppunt", "version": "1.0.0"}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "ok", "service": "nuts-knooppunt"}`))
	})

	// Mount subsystem handlers
	subsystemHandler := manager.CreateHandler()
	mux.Handle("/nuts/", subsystemHandler)

	// Test root endpoint
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test health endpoint
	req = httptest.NewRequest("GET", "/health", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test nuts subsystem endpoint
	req = httptest.NewRequest("GET", "/nuts/", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	expected := `{"message": "Nuts node public API", "path": "/"}`
	if w.Body.String() != expected {
		t.Errorf("Expected body %s, got %s", expected, w.Body.String())
	}

	// Test nuts health endpoint
	req = httptest.NewRequest("GET", "/nuts/health", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	expectedHealth := `{"status": "ok", "service": "nuts-node-public"}`
	if w.Body.String() != expectedHealth {
		t.Errorf("Expected body %s, got %s", expectedHealth, w.Body.String())
	}

	// Cleanup
	if err := manager.Stop(nil); err != nil {
		t.Fatalf("Failed to stop subsystems: %v", err)
	}
}