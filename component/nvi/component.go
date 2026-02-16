package nvi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/component/pseudonimization"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/profile"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tenants"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func DefaultConfig() Config {
	return Config{
		Audience: "nvi",
	}
}

type Config struct {
	FHIRBaseURL string `koanf:"baseurl"`
	Audience    string `koanf:"audience"`
}

func (c Config) Enabled() bool {
	return c.FHIRBaseURL != ""
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	pseudonymizer pseudonimization.Pseudonymizer
	audience      string
	fhirBaseURL   *url.URL
	fhirClientFn  func(ctx context.Context, uraNumber string) (fhirclient.Client, error)
}

func New(config Config, httpClientFn authn.HTTPClientProvider) (*Component, error) {
	baseURL, err := url.Parse(config.FHIRBaseURL)
	if err != nil {
		return nil, err
	}
	if config.Audience == "" {
		return nil, fmt.Errorf("audience must be configured when NVI component is enabled")
	}
	return &Component{
		fhirBaseURL: baseURL,
		fhirClientFn: func(ctx context.Context, uraNumber string) (fhirclient.Client, error) {
			// TODO: Cache the HTTP client per URA number, instead of creating a new one for each request.
			//       That would also allow caching the access tokens obtained from the OAuth2 server.
			httpClient, err := httpClientFn(ctx, []string{"epd:read", "epd:write"}, uraNumber, baseURL.String())
			if err != nil {
				return nil, err
			}
			httpClient.Transport = tracing.WrapTransport(httpClient.Transport)
			return fhirclient.New(baseURL, httpClient, fhirutil.ClientConfig()), nil
		},
		pseudonymizer: &pseudonimization.Component{},
		audience:      config.Audience,
	}, nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.Handle("POST /nvi/DocumentReference", http.HandlerFunc(c.handleRegister))
	internalMux.Handle("GET /nvi/DocumentReference", http.HandlerFunc(c.handleSearch))
	internalMux.Handle("POST /nvi/DocumentReference/_search", http.HandlerFunc(c.handleSearch))
}

func (c Component) handleRegister(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	requesterURA, err := tenants.IDFromRequest(httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirRequest, err := fhirapi.ParseRequest[fhir.DocumentReference](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	resource := fhirRequest.Resource

	// Make sure the right profile is set
	resource.Meta = profile.Set(resource.Meta, profile.NLGenericFunctionDocumentReference)

	// Use BSN transport tokens to NVI, instead of BSNs
	tokenizedResource, err := c.tokenizeIdentifiers(resource, c.audience)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	resource = *tokenizedResource

	fhirClient, err := c.fhirClientFn(httpRequest.Context(), *requesterURA.Value)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	// TODO: Remove this after migrating to real NVI
	requestHeaders := http.Header{
		"X-Requester-URA": []string{*requesterURA.Value},
	}
	var created fhir.DocumentReference
	err = fhirClient.CreateWithContext(httpRequest.Context(), resource, &created, fhirclient.RequestHeaders(requestHeaders))
	if err != nil {
		err = &fhirapi.Error{
			Message:   "Failed to register DocumentReference at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	// Translate BSN transport tokens from NVI back to BSNs
	result, err := c.detokenizeIdentifiers(created, *requesterURA.Value)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusCreated, result)
}

func (c Component) handleSearch(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	requesterURA, err := tenants.IDFromRequest(httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirRequest, err := fhirapi.ParseRequest[fhir.DocumentReference](httpRequest)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	// Use BSN transport tokens to NVI, instead of BSNs
	searchParams := url.Values{}
	for key, values := range fhirRequest.Parameters {
		newValues := append([]string{}, values...)
		if key == "patient:identifier" ||
			key == "subject:identifier" ||
			strings.HasPrefix(key, coding.BSNNamingSystem) {
			for i, value := range values {
				newValue, err := c.tokenizeFHIRSearchToken(value, "nvi")
				if err != nil {
					fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
					return
				}
				newValues[i] = newValue
			}
		}
		searchParams[key] = newValues
	}

	fhirClient, err := c.fhirClientFn(httpRequest.Context(), *requesterURA.Value)
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}
	requestHeaders := http.Header{
		"X-Requester-URA": []string{*requesterURA.Value},
	}
	var searchSet fhir.Bundle
	err = fhirClient.SearchWithContext(httpRequest.Context(), "DocumentReference", searchParams, &searchSet, fhirclient.RequestHeaders(requestHeaders))
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

	// Translate BSN transport tokens from NVI back to BSNs
	err = fhirutil.VisitBundleResources[fhir.DocumentReference](&searchSet, func(resource *fhir.DocumentReference) error {
		// Translate BSN transport tokens from NVI back to BSNs
		newResource, err := c.detokenizeIdentifiers(*resource, *requesterURA.Value)
		if err != nil {
			return err
		}
		*resource = *newResource
		return nil
	})
	if err != nil {
		fhirapi.SendErrorResponse(httpRequest.Context(), httpResponse, err)
		return
	}

	fhirapi.SendResponse(httpRequest.Context(), httpResponse, http.StatusOK, searchSet)
}

func (c Component) detokenizeIdentifiers(resource fhir.DocumentReference, audience string) (*fhir.DocumentReference, error) {
	if resource.Subject == nil || resource.Subject.Identifier == nil {
		return &resource, nil
	}
	detokenizedIdentifier, err := c.identifierToBSN(*resource.Subject.Identifier, audience)
	if err != nil {
		return nil, err
	}
	resource.Subject.Identifier = detokenizedIdentifier
	return &resource, nil
}

func (c Component) tokenizeIdentifiers(resource fhir.DocumentReference, audience string) (*fhir.DocumentReference, error) {
	if resource.Subject == nil || resource.Subject.Identifier == nil {
		return &resource, nil
	}
	tokenizedIdentifier, err := c.identifierToToken(*resource.Subject.Identifier, audience)
	if err != nil {
		return nil, err
	}
	resource.Subject.Identifier = tokenizedIdentifier
	return &resource, nil
}

// tokenizeFHIRSearchToken converts a FHIR search token  (<system>|<value>) to a BSN transport token value.
func (c Component) tokenizeFHIRSearchToken(searchToken string, audience string) (string, error) {
	if !strings.HasPrefix(searchToken, coding.BSNNamingSystem+"|") {
		return searchToken, nil
	}
	parts := strings.SplitN(searchToken, "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid FHIR search token: %s", searchToken)
	}
	identifier := fhir.Identifier{
		System: to.Ptr(parts[0]),
		Value:  to.Ptr(parts[1]),
	}
	tokenizedIdentifier, err := c.identifierToToken(identifier, audience)
	if err != nil {
		return "", err
	}
	return *tokenizedIdentifier.System + "|" + *tokenizedIdentifier.Value, nil
}

func (c Component) identifierToBSN(identifier fhir.Identifier, audience string) (*fhir.Identifier, error) {
	result, err := c.pseudonymizer.TokenToBSN(identifier, audience)
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to get BSN from transport token",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return result, nil
}

func (c Component) identifierToToken(identifier fhir.Identifier, audience string) (*fhir.Identifier, error) {
	result, err := c.pseudonymizer.IdentifierToToken(identifier, audience)
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to pseudonymize BSN identifier",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return result, nil
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
