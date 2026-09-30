package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

func TestCaptureActionDistinguishesStatusCleanupAndCategories(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		if r.URL.Path == "/nvi/List/_search" {
			_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[]}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"resourceType":"List","status":"current","mode":"working"}`))
	}))
	defer backend.Close()
	base, _ := url.Parse(backend.URL)
	lookup, register := nviFuncs(base, pool.PlataanClientID)
	var events []stepEvent
	ctx := withEventAction(withEventCapture(context.Background(), func(event stepEvent) {
		events = append(events, event)
	}, false), "share")
	cfg := Config{nviLookup: lookup}
	_ = cfg.patientShareStatus(ctx, "999900006")
	if err := register(ctx, "999900006", []string{nvi.CategoryPatient, nvi.CategoryCondition, nvi.CategoryMedicationRequest}); err != nil {
		t.Fatal(err)
	}
	_ = cfg.patientShareStatus(ctx, "999900006")
	if len(events) != 6 {
		t.Fatalf("captured %d calls, want 6", len(events))
	}
	wantPurpose := []string{"status", "registration-cleanup", "registration", "registration", "registration", "status"}
	wantResource := []string{"", "", "Patient", "Condition", "MedicationRequest", ""}
	seen := make(map[string]bool)
	for i, event := range events {
		if event.Action != "share" || event.ActionID == "" || event.ActionID != events[0].ActionID {
			t.Errorf("call %d lost its action: %+v", i, event)
		}
		if event.CallID == "" || seen[event.CallID] {
			t.Errorf("call %d has missing/repeated identity %q", i, event.CallID)
		}
		seen[event.CallID] = true
		if event.Purpose != wantPurpose[i] || event.ResourceType != wantResource[i] {
			t.Errorf("call %d purpose/category = %q/%q", i, event.Purpose, event.ResourceType)
		}
	}
}

func TestCaptureActionMetadataRejectsUnrecognizedText(t *testing.T) {
	var event stepEvent
	ctx := withEventCapture(context.Background(), func(value stepEvent) { event = value }, false)
	ctx = withEventPurpose(withEventAction(ctx, "secret-action"), "secret-purpose")
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.test/nvi/List/_search", nil)
	transport := newCaptureTransport("localization", captureRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
	}))
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if event.Action != "" || event.Purpose != "localization" {
		raw, _ := json.Marshal(event)
		t.Fatalf("unrecognized metadata was retained: %s", raw)
	}
}

func TestCaptureSourceQueriesKeepTheirKeyAndCategoryAcrossPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer source-issued-private-key" {
			t.Error("the source did not receive the issued key")
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		switch r.URL.Path {
		case "/Patient/_search":
			if r.Method != "POST" {
				t.Error("the patient identifier must travel in the search body")
			}
			_, _ = fmt.Fprintf(w, `{"resourceType":"Bundle","type":"searchset","link":[{"relation":"next","url":"http://%s/continuation"}]}`, r.Host)
		case "/continuation":
			_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":{"resourceType":"Patient","id":"source-patient"}}]}`))
		default:
			if r.Method != "GET" || r.URL.Query().Get("patient") != "Patient/source-patient" {
				t.Error("clinical query was not scoped to the source patient")
			}
			_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[]}`))
		}
	}))
	defer server.Close()
	var events []stepEvent
	ctx := withEventAction(withEventCapture(context.Background(), func(event stepEvent) { events = append(events, event) }, false), "retrieve")
	result, err := retrieveBGZ(ctx, sourceAddress{Address: server.URL}, "source-issued-private-key", pool.Patients()[0].BSN,
		[]string{nvi.CategoryAllergyIntolerance, nvi.CategoryCondition, nvi.CategoryMedicationRequest})
	if err != nil || !result.Granted() {
		t.Fatalf("retrieval failed: %v", err)
	}
	var categories, methods []string
	for _, event := range events {
		categories = append(categories, event.ResourceType)
		methods = append(methods, event.Request.Method)
		if !event.Request.TokenAttached || event.Action != "retrieve" {
			t.Fatal("source request lost its action or key evidence")
		}
	}
	if !slices.Equal(categories, []string{"Patient", "Patient", "AllergyIntolerance", "Condition", "MedicationRequest"}) {
		t.Fatalf("query categories: %v", categories)
	}
	if !slices.Equal(methods, []string{"POST", "GET", "GET", "GET", "GET"}) {
		t.Fatalf("query methods: %v", methods)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "source-issued-private-key") {
		t.Fatal("source credential leaked into retained events")
	}
}

func TestCaptureDiscoveryAndRetrievalAreSeparateActions(t *testing.T) {
	for _, tc := range []struct{ method, want string }{{"GET", "discover"}, {"POST", "retrieve"}} {
		req := httptest.NewRequest(tc.method, "/demo/ehr/patients/anna/retrieve", nil)
		ctx := withEventAction(req.Context(), actionForRequest(req))
		event := stepEvent{GF: "addressing"}
		applyEventAction(ctx, &event)
		if event.Action != tc.want || event.ActionID == "" {
			t.Fatalf("%s action: %q", tc.method, event.Action)
		}
	}
}
