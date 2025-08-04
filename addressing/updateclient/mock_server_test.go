package updateclient_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
)

// TestServer represents a test HTTP server that serves JSON responses from files
type TestServer struct {
	Server *httptest.Server
}

// NewTestServer creates a new test server with handlers configured to serve JSON files
func NewTestServer() *TestServer {
	ts := &TestServer{}

	// Create a handler that will serve JSON files from testdata directory
	handler := http.NewServeMux()
	handler.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		// Default to 404
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `OK`)
	})

	// Create and start the test server
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(os.Stdout, "TestServer: ", log.LstdFlags)
	ts.Server = server

	return ts
}

// AddJSONFileHandler adds a handler for a specific path to serve a JSON file
func (ts *TestServer) AddJSONFileHandler(path, jsonFileName string) {
	handler := ts.Server.Config.Handler.(*http.ServeMux)

	handler.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		// Set content type header
		w.Header().Set("Content-Type", "application/json")

		// Read the JSON file
		filePath := filepath.Join("testdata", jsonFileName)
		fmt.Println("Reading JSON file:", filePath)
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Println("Error reading JSON file:", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error": "Failed to read JSON file: %s"}`, err.Error())
			return
		}

		// Write the response
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}

// AddCustomHandler adds a handler that returns a custom status code and response
func (ts *TestServer) AddCustomHandler(path string, statusCode int, response string) {
	handler := ts.Server.Config.Handler.(*http.ServeMux)

	handler.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, response)
	})
}

// AddCustomHandlerFunc adds a handler with a custom handler function
func (ts *TestServer) AddCustomHandlerFunc(path string, handlerFunc http.HandlerFunc) {
	handler := ts.Server.Config.Handler.(*http.ServeMux)
	handler.HandleFunc(path, handlerFunc)
}

// AddRequestValidationHandler adds a handler that validates the request and returns a response
func (ts *TestServer) AddRequestValidationHandler(path string, validator func(*http.Request) (int, string)) {
	handler := ts.Server.Config.Handler.(*http.ServeMux)

	handler.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		statusCode, response := validator(r)
		w.WriteHeader(statusCode)
		fmt.Fprint(w, response)
	})
}

// URL returns the base URL of the test server
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// Close stops the test server
func (ts *TestServer) Close() {
	ts.Server.Close()
}

func (ts *TestServer) Start() {
	ts.Server.Start()
}
