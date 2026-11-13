package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestEnrichResourceContent(t *testing.T) {
	t.Run("fetches resource content via full PIP pipeline", func(t *testing.T) {
		task := fhir.Task{
			Id:     to.Ptr("task-1"),
			Status: fhir.TaskStatusRequested,
			Owner: &fhir.Reference{
				Identifier: &fhir.Identifier{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
					Value:  to.Ptr("00000001"),
				},
			},
		}

		component := &Component{
			Config: Config{PIP: PIPConfig{ResourceContentEnabled: true}},
			pipClient: &test.StubFHIRClient{
				Resources: []any{task},
			},
		}

		resourceType := fhir.ResourceTypeTask
		input := &PolicyInput{
			Resource: PolicyResource{
				Type: &resourceType,
				Id:   "task-1",
			},
		}

		result, _ := component.enrichPolicyInputWithPIP(context.Background(), input)

		require.NotNil(t, result.Resource.Content, "resource content should be populated after PIP enrichment")
		assert.Equal(t, "task-1", result.Resource.Content["id"])
		assert.Equal(t, "requested", result.Resource.Content["status"])
		assert.Equal(t, "Task", result.Resource.Content["resourceType"])

		owner, ok := result.Resource.Content["owner"].(map[string]any)
		require.True(t, ok)
		identifier, ok := owner["identifier"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/ura", identifier["system"])
		assert.Equal(t, "00000001", identifier["value"])
	})

	t.Run("skips when no resource type", func(t *testing.T) {
		component := &Component{
			pipClient: &test.StubFHIRClient{},
		}
		input := &PolicyInput{
			Resource: PolicyResource{Id: "task-1"},
		}

		result, reasons := component.enrichResourceContent(context.Background(), input)
		assert.Empty(t, reasons)
		assert.Nil(t, result.Resource.Content)
	})

	t.Run("skips when no resource id", func(t *testing.T) {
		component := &Component{
			pipClient: &test.StubFHIRClient{},
		}
		resourceType := fhir.ResourceTypeTask
		input := &PolicyInput{
			Resource: PolicyResource{Type: &resourceType},
		}

		result, reasons := component.enrichResourceContent(context.Background(), input)
		assert.Empty(t, reasons)
		assert.Nil(t, result.Resource.Content)
	})

	t.Run("returns pip_error and logs no resource id when resource not found", func(t *testing.T) {
		handler := &captureHandler{}
		originalDefault := slog.Default()
		slog.SetDefault(slog.New(handler))
		defer slog.SetDefault(originalDefault)

		component := &Component{
			pipClient: &test.StubFHIRClient{},
		}
		resourceType := fhir.ResourceTypeTask
		input := &PolicyInput{
			Resource: PolicyResource{
				Type: &resourceType,
				Id:   "nonexistent",
			},
		}

		result, reasons := component.enrichResourceContent(context.Background(), input)
		require.Len(t, reasons, 1)
		assert.Equal(t, TypeResultCodePIPError, reasons[0].Code)
		assert.Nil(t, result.Resource.Content)

		// Verify the log names the resource type, but not the resource id (it may identify a patient)
		var loggedResourceType string
		for _, record := range handler.records {
			record.Attrs(func(attr slog.Attr) bool {
				if attr.Key == "resource_type" {
					loggedResourceType = attr.Value.String()
				}
				assert.NotContains(t, attr.Value.String(), "nonexistent")
				return true
			})
		}
		assert.Equal(t, "Task", loggedResourceType)
	})
}

// TestEnrichPolicyInputWithPIP_NoPatientDataInLogsOrReasons runs every PIP lookup against a PIP
// that fails, and checks that the patient's id (in the request paths and queries) ends up neither
// in the log nor in the result reasons.
func TestEnrichPolicyInputWithPIP_NoPatientDataInLogsOrReasons(t *testing.T) {
	const patientID = "patient-900186021"
	enrich := func(t *testing.T, pipURL string) (logOutput string, reasons []ResultReason) {
		t.Helper()
		var logs bytes.Buffer
		originalDefault := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { slog.SetDefault(originalDefault) })

		baseURL, err := url.Parse(pipURL)
		require.NoError(t, err)
		component := &Component{
			Config:    Config{PIP: PIPConfig{ResourceContentEnabled: true}},
			pipClient: fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{}),
		}
		resourceType := fhir.ResourceTypePatient
		input := &PolicyInput{
			Resource: PolicyResource{Type: &resourceType, Id: patientID},
			Action:   PolicyAction{ConnectionTypeCode: "hl7-fhir-rest"},
			Context:  PolicyContext{PatientID: patientID},
		}
		_, reasons = component.enrichPolicyInputWithPIP(context.Background(), input)
		return logs.String(), reasons
	}
	assertNoPatientData := func(t *testing.T, logOutput string, reasons []ResultReason) {
		t.Helper()
		require.Len(t, reasons, 3, "the patient, consent and resource content lookups all fail")
		assert.Contains(t, logOutput, "Failed to get patient identifiers from PIP")
		assert.Contains(t, logOutput, "PIP consent retrieval failed")
		assert.Contains(t, logOutput, "Failed to get resource content from PIP")
		assert.NotContains(t, logOutput, "900186021")
		for _, reason := range reasons {
			assert.NotContains(t, reason.Description, "900186021")
		}
	}

	t.Run("PIP answers with an OperationOutcome that echoes the request", func(t *testing.T) {
		pip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/fhir+json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{{
				Severity:    fhir.IssueSeverityError,
				Code:        fhir.IssueTypeInvalid,
				Diagnostics: to.Ptr("Invalid request: " + r.URL.String()),
			}}})
		}))
		t.Cleanup(pip.Close)

		logOutput, reasons := enrich(t, pip.URL)

		assertNoPatientData(t, logOutput, reasons)
	})
	t.Run("PIP is unreachable", func(t *testing.T) {
		pip := httptest.NewServer(http.NotFoundHandler())
		pip.Close()

		logOutput, reasons := enrich(t, pip.URL)

		assertNoPatientData(t, logOutput, reasons)
	})
}
