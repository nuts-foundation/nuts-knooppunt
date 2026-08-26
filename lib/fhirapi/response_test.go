package fhirapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestStatusCodeForError(t *testing.T) {
	t.Run("invalid issue type maps to 400", func(t *testing.T) {
		err := &Error{Message: "bad input", IssueType: fhir.IssueTypeInvalid}
		assert.Equal(t, http.StatusBadRequest, StatusCodeForError(err))
	})

	t.Run("transient issue type maps to 503", func(t *testing.T) {
		err := &Error{Message: "upstream unavailable", IssueType: fhir.IssueTypeTransient}
		assert.Equal(t, http.StatusServiceUnavailable, StatusCodeForError(err))
	})

	t.Run("too costly issue type maps to 422", func(t *testing.T) {
		err := &Error{Message: "too many results", IssueType: fhir.IssueTypeTooCostly}
		assert.Equal(t, http.StatusUnprocessableEntity, StatusCodeForError(err))
	})

	t.Run("unmapped issue type maps to 500", func(t *testing.T) {
		err := &Error{Message: "not authorized", IssueType: fhir.IssueTypeSecurity}
		assert.Equal(t, http.StatusInternalServerError, StatusCodeForError(err))
	})

	t.Run("non-Error maps to 500", func(t *testing.T) {
		assert.Equal(t, http.StatusInternalServerError, StatusCodeForError(errors.New("database connection failed")))
	})
}

func TestOperationOutcomeForError(t *testing.T) {
	t.Run("Error yields its own OperationOutcome", func(t *testing.T) {
		err := &Error{Message: "Invalid resource format", Cause: errors.New("validation failed"), IssueType: fhir.IssueTypeInvalid}

		outcome := OperationOutcomeForError(err)

		assert.Equal(t, err.OperationOutcome(), outcome)
		assert.Equal(t, "Invalid resource format", *outcome.Issue[0].Diagnostics)
		assert.Equal(t, fhir.IssueTypeInvalid, outcome.Issue[0].Code)
	})

	t.Run("generic error yields a diagnostics-free OperationOutcome", func(t *testing.T) {
		outcome := OperationOutcomeForError(errors.New("database connection failed"))

		assert.Equal(t, fhir.IssueTypeProcessing, outcome.Issue[0].Code)
		assert.Equal(t, "An internal server error occurred", *outcome.Issue[0].Diagnostics)
	})
}
