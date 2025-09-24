package fhirapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// SendErrorResponse will send the given error as OperationOutcome to the FHIR client.
// If the error isn't an Error instance, it will send a generic error back to the FHIR client, to avoid leaking sensitive internals.
func SendErrorResponse(ctx context.Context, httpResponse http.ResponseWriter, err error) {
	log.Ctx(ctx).Err(err).Msgf("FHIR API error")
	statusCode := http.StatusInternalServerError
	var responseResource any
	var fhirError *Error
	if ok := errors.As(err, &fhirError); ok {
		// Might want to support more later, not required now
		switch fhirError.IssueType {
		case fhir.IssueTypeInvalid,
			fhir.IssueTypeStructure,
			fhir.IssueTypeRequired,
			fhir.IssueTypeValue,
			fhir.IssueTypeInvariant:
			statusCode = http.StatusBadRequest
		case fhir.IssueTypeTransient,
			fhir.IssueTypeLockError,
			fhir.IssueTypeNoStore,
			fhir.IssueTypeException,
			fhir.IssueTypeTimeout,
			fhir.IssueTypeThrottled:
			statusCode = http.StatusServiceUnavailable
		case fhir.IssueTypeTooCostly:
			statusCode = http.StatusUnprocessableEntity
		}
		responseResource = fhirError.OperationOutcome()
	} else {
		diagnostics := "An internal server error occurred"
		responseResource = fhir.OperationOutcome{
			Issue: []fhir.OperationOutcomeIssue{
				{
					Severity:    fhir.IssueSeverityError,
					Code:        fhir.IssueTypeProcessing,
					Diagnostics: &diagnostics,
				},
			},
		}
	}
	SendResponse(ctx, httpResponse, statusCode, responseResource)
}

func SendResponse(ctx context.Context, httpResponse http.ResponseWriter, httpStatus int, resource interface{}) {
	data, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		log.Ctx(ctx).Err(err).Msg("Failed to marshal response")
		httpStatus = http.StatusInternalServerError
		data = []byte(`{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"processing","diagnostics":"Failed to marshal response"}]}`)
	}
	httpResponse.Header().Set("Content-Type", JSONMimeType)
	httpResponse.Header().Set("Content-Length", strconv.Itoa(len(data)))
	httpResponse.WriteHeader(httpStatus)
	_, err = httpResponse.Write(data)
	if err != nil {
		log.Ctx(ctx).Err(err).Msgf("Failed to write response: %s", string(data))
	}
}
