package nvi

import (
	"context"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func DefaultConfig() Config {
	return Config{}
}

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
		client: fhirclient.New(baseURL, tracing.NewHTTPClient(), fhirutil.ClientConfig()),
	}, nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.Handle("POST /nvi/Bundle", http.HandlerFunc(c.handleRegister))
	internalMux.Handle("GET /nvi/List", http.HandlerFunc(c.handleSearch))
	internalMux.Handle("POST /nvi/List/_search", http.HandlerFunc(c.handleSearch))
}

func (c Component) handleRegister(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ParseRequest[fhir.Bundle](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	bundle := fhirRequest.Resource

	if bundle.Type != fhir.BundleTypeTransaction {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, &fhirapi.Error{
			Message:   "Bundle must be of type transaction",
			IssueType: fhir.IssueTypeValue,
		})
		return
	}

	/**
	todo: pseduoanonymization
	*/

	var result fhir.Bundle
	err = c.client.CreateWithContext(httpRequest.Context(), bundle, &result)
	if err != nil {
		err = &fhirapi.Error{
			Message:   "Failed to register Bundle at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusCreated, result)
}

func (c Component) handleSearch(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	fhirRequest, err := fhirapi.ParseRequest[fhir.List](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	searchParams := url.Values{}
	for key, values := range fhirRequest.Parameters {
		searchParams[key] = append([]string{}, values...)
	}
	/**
	todo: pseduoanonymization
	*/

	var searchSet fhir.Bundle
	err = c.client.SearchWithContext(httpRequest.Context(), "List", searchParams, &searchSet)
	if err != nil {
		err = &fhirapi.Error{
			Message:   "Failed to search for List resources at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	if hasNextLink(&searchSet) {
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
