package nvi

import (
	"context"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/profile"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Config struct {
	FHIRBaseURL string `koanf:"baseurl"`
}

func (c Config) Enabled() bool {
	return c.FHIRBaseURL != ""
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	client fhirclient.Client
}

func New(config Config) (*Component, error) {
	baseURL, err := url.Parse(config.FHIRBaseURL)
	if err != nil {
		return nil, err
	}
	return &Component{
		client: fhirclient.New(baseURL, http.DefaultClient, fhirutil.ClientConfig()),
	}, nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.Handle("POST /nvi/DocumentReference", http.HandlerFunc(c.handleRegister))
	internalMux.Handle("POST /nvi/DocumentReference/_search", http.HandlerFunc(c.handleSearch))
}

func (c Component) handleRegister(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ReadRequest[fhir.DocumentReference](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	// Make sure the right profile is set
	fhirRequest.Resource.Meta = profile.Set(fhirRequest.Resource.Meta, profile.NLGenericFunctionDocumentReference)

	var created fhir.DocumentReference
	err = c.client.CreateWithContext(httpRequest.Context(), fhirRequest.Resource, &created)
	if err != nil {
		err = &fhirapi.Error{
			Message:   "Failed to register DocumentReference at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusCreated, created)
}

func (c Component) handleSearch(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ReadRequest[fhir.DocumentReference](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	var searchSet fhir.Bundle
	err = c.client.SearchWithContext(httpRequest.Context(), "DocumentReference", fhirRequest.Parameters, &searchSet)
	if err != nil {
		err = &fhirapi.Error{
			Message:   "Failed to search for DocumentReferences at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	if hasNextLink(&searchSet) {
		// Otherwise must paginate, not supported for now.
		err = &fhirapi.Error{
			Message:   "NVI returned more results than can be handled. Please refine your search, or increase _count.",
			IssueType: fhir.IssueTypeTooCostly,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusOK, searchSet)
}

func (c Component) Start() error {
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	return nil
}

func hasNextLink(bundle *fhir.Bundle) bool {
	if bundle.Link == nil {
		return false
	}
	for _, link := range bundle.Link {
		if link.Relation == "next" {
			return true
		}
	}
	return false
}
