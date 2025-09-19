package fhirapi

import (
	"fmt"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Error defines a problem that can be translated to a FHIR OperationOutcome, to be returned to the FHIR client.
type Error struct {
	// Message is the message that can be return to the FHIR client.
	Message string
	// Cause is an optional error that is only logged internally, not returned to the FHIR client.
	Cause error
	// IssueType is the FHIR issue type that is used in the OperationOutcome.
	IssueType fhir.IssueType
}

func (e Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e Error) OperationOutcome() fhir.OperationOutcome {
	return fhir.OperationOutcome{
		Issue: []fhir.OperationOutcomeIssue{
			{
				Severity:    fhir.IssueSeverityError,
				Code:        e.IssueType,
				Diagnostics: &e.Message,
			},
		},
	}
}

func BadRequestError(message string, cause error) error {
	return &Error{
		Message:   message,
		Cause:     cause,
		IssueType: fhir.IssueTypeInvalid,
	}
}
