package subsystems

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubsystemsComponent(t *testing.T) {
	// Create the subsystems component
	component := New()

	// Start the component 
	err := component.Start()
	if err != nil {
		t.Fatalf("Failed to start subsystems component: %v", err)
	}

	// Give time for subsystems to start
	time.Sleep(200 * time.Millisecond)

	// Create a test mux and register handlers
	mux := http.NewServeMux()
	component.RegisterHttpHandlers(mux)

	// Test that the subsystem route is registered
	req := httptest.NewRequest("GET", "/nuts/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for /nuts/health, got %d", w.Code)
	}

	expected := `{"status": "ok", "service": "nuts-node-public"}`
	if w.Body.String() != expected {
		t.Errorf("Expected body %s, got %s", expected, w.Body.String())
	}

	// Test root nuts endpoint
	req = httptest.NewRequest("GET", "/nuts/", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for /nuts/, got %d", w.Code)
	}

	expectedRoot := `{"message": "Nuts node public API", "path": "/"}`
	if w.Body.String() != expectedRoot {
		t.Errorf("Expected body %s, got %s", expectedRoot, w.Body.String())
	}

	// Stop the component
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = component.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop subsystems component: %v", err)
	}
}