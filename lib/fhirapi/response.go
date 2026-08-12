package fhirapi

import (
	"errors"
	"net/http"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// StatusCodeForError maps err to the HTTP status code its OperationOutcome should be sent with:
// http.StatusInternalServerError unless err is an *Error with an IssueType mapped below.
func StatusCodeForError(err error) int {
	var fhirError *Error
	if ok := errors.As(err, &fhirError); ok {
		// Might want to support more later, not required now
		switch fhirError.IssueType {
		case fhir.IssueTypeInvalid,
			fhir.IssueTypeStructure,
			fhir.IssueTypeRequired,
			fhir.IssueTypeValue,
			fhir.IssueTypeInvariant:
			return http.StatusBadRequest
		case fhir.IssueTypeTransient,
			fhir.IssueTypeLockError,
			fhir.IssueTypeNoStore,
			fhir.IssueTypeException,
			fhir.IssueTypeTimeout,
			fhir.IssueTypeThrottled:
			return http.StatusServiceUnavailable
		case fhir.IssueTypeTooCostly:
			return http.StatusUnprocessableEntity
		}
	}
	return http.StatusInternalServerError
}

// OperationOutcomeForError builds the FHIR OperationOutcome body to send for err: err's own
// OperationOutcome() if it's an *Error, otherwise a generic one that avoids leaking internal
// details.
func OperationOutcomeForError(err error) fhir.OperationOutcome {
	var fhirError *Error
	if ok := errors.As(err, &fhirError); ok {
		return fhirError.OperationOutcome()
	}
	diagnostics := "An internal server error occurred"
	return fhir.OperationOutcome{
		Issue: []fhir.OperationOutcomeIssue{
			{
				Severity:    fhir.IssueSeverityError,
				Code:        fhir.IssueTypeProcessing,
				Diagnostics: &diagnostics,
			},
		},
	}
}
