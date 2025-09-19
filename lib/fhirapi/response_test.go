package fhirapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestSendResponse(t *testing.T) {
	t.Run("send response", func(t *testing.T) {
		resource := map[string]interface{}{
			"resourceType": "Patient",
			"id":           "123",
			"name": []map[string]interface{}{
				{"family": "Doe", "given": []string{"John"}},
			},
		}

		recorder := httptest.NewRecorder()
		ctx := context.Background()
		SendResponse(ctx, recorder, http.StatusOK, resource)

		// Check status code
		assert.Equal(t, http.StatusOK, recorder.Code)

		// Check headers
		assert.Equal(t, JSONMimeType, recorder.Header().Get("Content-Type"))
		expectedLength := len(recorder.Body.Bytes())
		assert.Equal(t, strconv.Itoa(expectedLength), recorder.Header().Get("Content-Length"))

		// Check response body contains expected JSON
		body := recorder.Body.String()
		assert.Contains(t, body, `"resourceType": "Patient"`)
		assert.Contains(t, body, `"id": "123"`)
		assert.Contains(t, body, `"family": "Doe"`)
	})
}

func TestSendErrorResponse(t *testing.T) {
	t.Run("API error", func(t *testing.T) {
		// Test FHIR API error that should return 400 Bad Request
		apiError := &Error{
			Message:   "Invalid resource format",
			Cause:     errors.New("validation failed"),
			IssueType: fhir.IssueTypeInvalid,
		}

		recorder := httptest.NewRecorder()
		ctx := context.Background()

		SendErrorResponse(ctx, recorder, apiError)

		// Check status code - should be 400 for invalid issue types
		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		// Check headers
		assert.Equal(t, JSONMimeType, recorder.Header().Get("Content-Type"))
		expectedLength := len(recorder.Body.Bytes())
		assert.Equal(t, strconv.Itoa(expectedLength), recorder.Header().Get("Content-Length"))

		// Check response body contains OperationOutcome with the API error details
		body := recorder.Body.String()
		assert.Contains(t, body, `"resourceType": "OperationOutcome"`)
		assert.Contains(t, body, `"severity": "error"`)
		assert.Contains(t, body, `"code": "invalid"`)
		assert.Contains(t, body, `"diagnostics": "Invalid resource format"`)
	})

	t.Run("other error", func(t *testing.T) {
		// Test generic error that should return 500 Internal Server Error
		genericError := errors.New("database connection failed")

		recorder := httptest.NewRecorder()
		ctx := context.Background()

		SendErrorResponse(ctx, recorder, genericError)

		// Check status code - should be 500 for generic errors
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)

		// Check headers
		assert.Equal(t, JSONMimeType, recorder.Header().Get("Content-Type"))
		expectedLength := len(recorder.Body.Bytes())
		assert.Equal(t, strconv.Itoa(expectedLength), recorder.Header().Get("Content-Length"))

		// Check response body contains generic OperationOutcome
		body := recorder.Body.String()
		assert.Contains(t, body, `"resourceType": "OperationOutcome"`)
		assert.Contains(t, body, `"severity": "error"`)
		assert.Contains(t, body, `"code": "processing"`)
		assert.Contains(t, body, `"diagnostics": "An internal server error occurred"`)
	})
}
