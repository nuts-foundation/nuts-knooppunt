package fhirapi

import (
	"encoding/json"
	"mime"
	"net/http"
)

const JSONMimeType = "application/fhir+json"

type Request[T any] struct {
	Resource T
}

// ReadRequest reads an HTTP request as FHIR request.
// If it fails, the returned error can be sent to the client as OperationOutcome.
func ReadRequest[T any](httpRequest *http.Request) (*Request[T], error) {
	mediaType, _, err := mime.ParseMediaType(httpRequest.Header.Get("Content-Type"))
	if err != nil {
		return nil, BadRequestError("invalid content type", err)
	}
	if mediaType != JSONMimeType {
		return nil, BadRequestError("invalid content type, expected application/fhir+json", nil)
	}
	var resource T
	err = json.NewDecoder(httpRequest.Body).Decode(&resource)
	if err != nil {
		return nil, BadRequestError("request body is not valid JSON", err)
	}
	return &Request[T]{
		Resource: resource,
	}, nil
}
