package fhirapi

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
)

const JSONMimeType = "application/fhir+json"
const FormMimeType = "application/x-www-form-urlencoded"

type Request[T any] struct {
	Resource   T
	Parameters url.Values
}

// ReadRequest reads an HTTP request as FHIR request.
// If it fails, the returned error can be sent to the client as OperationOutcome.
func ReadRequest[T any](httpRequest *http.Request) (*Request[T], error) {
	mediaType, _, err := mime.ParseMediaType(httpRequest.Header.Get("Content-Type"))
	if err != nil {
		return nil, BadRequestError("invalid content type", err)
	}

	var request *Request[T]
	switch mediaType {
	case JSONMimeType:
		var resource T
		err = json.NewDecoder(httpRequest.Body).Decode(&resource)
		if err != nil {
			return nil, BadRequestError("request body is not valid "+JSONMimeType, err)
		}
		request = &Request[T]{Resource: resource}
	case FormMimeType:
		err := httpRequest.ParseForm()
		if err != nil {
			return nil, BadRequestError("request body is not valid "+FormMimeType, err)
		}
		request = &Request[T]{Parameters: httpRequest.Form}
	default:
		return nil, BadRequestError("invalid content type", nil)
	}
	return request, nil
}
