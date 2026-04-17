package pdp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/policies"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/open-policy-agent/opa/v1/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// captureHandler is a slog.Handler that records all log messages for inspection.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Clone the record to avoid storing references to data that may be invalid after Handle returns.
	r = r.Clone()
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := make([]string, len(h.records))
	for i, r := range h.records {
		msgs[i] = r.Message
	}
	return msgs
}

// allText renders every captured record — message plus every attribute value —
// as a single string, so tests can grep for sensitive substrings that may
// appear either in the log message or in any structured attribute.
func (h *captureHandler) allText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var b strings.Builder
	for _, r := range h.records {
		b.WriteString(r.Message)
		b.WriteByte('\n')
		r.Attrs(func(a slog.Attr) bool {
			b.WriteString(a.Value.String())
			b.WriteByte('\n')
			return true
		})
	}
	return b.String()
}

func TestOPALogsGoThroughSlog(t *testing.T) {
	// Set up a capturing slog handler and bridge logrus to it
	handler := &captureHandler{}
	originalDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(originalDefault)
	logging.InitLogrus()

	// Start a test HTTP server to serve bundles
	mux := http.NewServeMux()
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	service, err := New(Config{Enabled: true}, nil)
	require.NoError(t, err)
	service.opaBundleBaseURL = httpServer.URL + "/pdp/bundles/"
	service.RegisterHttpHandlers(nil, mux)

	// Start OPA - this triggers bundle loading which produces log messages
	require.NoError(t, service.Start())
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	// OPA should have logged "Starting bundle loader." and "Bundle loaded and activated successfully."
	// messages through slog, not directly to stdout via logrus.
	msgs := handler.messages()
	hasBundleLog := false
	for _, msg := range msgs {
		if msg == "Starting bundle loader." || msg == "Bundle loaded and activated successfully." {
			hasBundleLog = true
			break
		}
	}
	assert.True(t, hasBundleLog,
		"expected OPA log messages to go through slog, but none were captured. Got messages: %v", msgs)
}

// TestOPADecisionLogsMaskBSN asserts that OPA's decision-log plugin finds and
// applies `data.system.log.mask` from the embedded `system` bundle: the BSN
// and other known BSN-bearing fields must not appear in the captured log
// stream, and the [REDACTED] placeholder must appear instead. This proves the
// whole chain (bundle load → decision → mask → ConsoleLogger → slog) works
// end-to-end, not just the rego rule in isolation.
func TestOPADecisionLogsMaskBSN(t *testing.T) {
	handler := &captureHandler{}
	originalDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(originalDefault)
	logging.InitLogrus()

	bundles, err := policies.Bundles(t.Context())
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /pdp/bundles/{policyName}", func(w http.ResponseWriter, r *http.Request) {
		policyName := strings.TrimSuffix(r.PathValue("policyName"), ".tar.gz")
		data, found := bundles[policyName]
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(data)
	})
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	opaService, err := createOPAService(t.Context(), httpServer.URL+"/pdp/bundles/", bundles)
	require.NoError(t, err)
	defer opaService.Stop(t.Context())

	// Build a fully-populated PolicyInput from the Go struct (not a hand-rolled
	// map) so this test fails if a future struct change adds a BSN-bearing
	// field that the mask rego doesn't cover. Every field that can carry a BSN
	// or BSN-scoped identifier must contain `bsnSentinel`; we then assert the
	// sentinel never appears anywhere in the captured log stream.
	const bsnSentinel = "900186021"
	patientType := fhir.ResourceTypePatient
	policyInput := PolicyInput{
		Context: PolicyContext{
			PatientBSN:   bsnSentinel,
			PatientID:    "patient-123",
			MitzConsent:  true,
			PurposeOfUse: "TREAT",
		},
		Subject: PolicySubject{
			OtherProps: OtherProps{
				"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|" + bsnSentinel,
			},
		},
		Resource: PolicyResource{
			Type: &patientType,
			Content: map[string]any{
				"resourceType": "Patient",
				"identifier": []map[string]any{
					{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": bsnSentinel},
				},
			},
		},
		Action: PolicyAction{
			Request: HTTPRequest{
				Method: "GET",
				Path:   "/Patient",
				Query:  "identifier=http://fhir.nl/fhir/NamingSystem/bsn|" + bsnSentinel,
				Body:   `{"identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"` + bsnSentinel + `"}]}`,
				Header: http.Header{"Authorization": []string{"Bearer token-" + bsnSentinel}},
			},
			FHIRRest: FHIRRestData{
				CapabilityChecked: true,
				Include:           []string{"Patient:general-practitioner"},
				SearchParams: map[string][][]string{
					"identifier": {{"http://fhir.nl/fhir/NamingSystem/bsn|" + bsnSentinel}},
				},
			},
		},
	}

	input, err := to.JSONMap(policyInput)
	require.NoError(t, err)
	_, err = opaService.Decision(t.Context(), sdk.DecisionOptions{Path: "/bgz", Input: input})
	require.NoError(t, err)

	captured := handler.allText()
	// Guard: the assertions below only prove anything if the decision-log
	// message actually landed in our capture buffer. "Decision Log" is the
	// literal log message emitted by OPA's console decision-log plugin.
	require.Contains(t, captured, "Decision Log",
		"decision log was not captured; the mask assertions below would be vacuously true")
	assert.NotContains(t, captured, bsnSentinel, "BSN must not appear anywhere in decision logs")
	assert.Contains(t, captured, "[REDACTED]", "expected at least one field to be redacted")
}
