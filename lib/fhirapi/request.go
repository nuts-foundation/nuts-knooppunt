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

// ParseRequest reads an HTTP request as FHIR request.
// If it fails, the returned error can be sent to the client as OperationOutcome.
func ParseRequest[T any](httpRequest *http.Request) (*Request[T], error) {
	request := Request[T]{
		Parameters: make(url.Values),
	}
	contentType := httpRequest.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, BadRequestError("invalid content type", err)
		}
		switch mediaType {
		case JSONMimeType:
			var resource T
			err = json.NewDecoder(httpRequest.Body).Decode(&resource)
			if err != nil {
				return nil, BadRequestError("request body is not valid "+JSONMimeType, err)
			}
			request.Resource = resource
		case FormMimeType:
			err := httpRequest.ParseForm()
			if err != nil {
				return nil, BadRequestError("request body is not valid "+FormMimeType, err)
			}
			request.Parameters = httpRequest.Form
		default:
			return nil, BadRequestError("invalid content type: "+mediaType, nil)
		}
	}
	// Parse URL parameters
	for key, values := range httpRequest.URL.Query() {
		for _, value := range values {
			request.Parameters.Add(key, value)
		}
	}

	return &request, nil
}
