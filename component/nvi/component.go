package nvi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/component/pseudonymisation"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
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
	pseudonymizer pseudonymisation.Pseudonymizer
	audience      string
	fhirBaseURL   *url.URL
	fhirClientFn  func(ctx context.Context, uraNumber string) (fhirclient.Client, error)
}

func New(config Config, httpClientFn authn.HTTPClientProvider, pseudonymizer pseudonymisation.Pseudonymizer) (*Component, error) {
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
			httpClient, err := httpClientFn(ctx, []string{"epd:read", "epd:write"}, uraNumber, baseURL.Scheme+"://"+baseURL.Host)
			if err != nil {
				return nil, err
			}
			httpClient.Transport = &loggingTransport{next: tracing.WrapTransport(httpClient.Transport)}
			clientConfig := fhirutil.ClientConfig()
			clientConfig.UsePostSearch = false // nvi doesn't support POST searches
			return fhirclient.New(baseURL, httpClient, clientConfig), nil
		},
		pseudonymizer: pseudonymizer,
		audience:      config.Audience,
	}, nil
}

// RegisterHttpHandlers registers no routes: every NVI operation is served through the generated
// OpenAPI strict server, wired up in strictAPIServer.RegisterHttpHandlers in package cmd.
func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
}

// RegisterBundle submits a FHIR transaction Bundle containing one or more `List` resources to
// NVI. BSN values in `List.subject.identifier` are pseudonymized (tokenized) before the Bundle
// is forwarded to the upstream NVI FHIR server.
func (c Component) RegisterBundle(ctx context.Context, tenantURA string, bundle fhir.Bundle) (*fhir.Bundle, error) {
	// Use BSN transport tokens to NVI, instead of BSNs
	for i, entry := range bundle.Entry {
		if entry.Resource == nil {
			continue
		}
		var resourceType struct {
			ResourceType string `json:"resourceType"`
		}
		if err := json.Unmarshal(entry.Resource, &resourceType); err != nil || resourceType.ResourceType != "List" {
			continue
		}
		var listResource fhir.List
		if err := json.Unmarshal(entry.Resource, &listResource); err != nil {
			continue
		}
		tokenizedList, err := c.tokenizeListIdentifiers(ctx, listResource, tenantURA, c.audience)
		if err != nil {
			return nil, err
		}
		tokenizedJSON, err := json.Marshal(tokenizedList)
		if err != nil {
			return nil, err
		}
		bundle.Entry[i].Resource = tokenizedJSON
	}

	fhirClient, err := c.fhirClientFn(ctx, tenantURA)
	if err != nil {
		return nil, err
	}

	var result fhir.Bundle
	err = fhirClient.CreateWithContext(ctx, bundle, &result, fhirclient.AtPath(""))
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to register Bundle at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return &result, nil
}

// registerList registers a `List` resource directly (not wrapped in a Bundle) at NVI. Named in
// lowercase to avoid colliding with the RegisterNVIList method required by api.StrictServerInterface
// (generated for the POST /nvi/List operation, named "registerNVIList" in openapi.yaml).
func (c Component) registerList(ctx context.Context, tenantURA string, list fhir.List) (*fhir.List, error) {
	tokenizedList, err := c.tokenizeListIdentifiers(ctx, list, tenantURA, c.audience)
	if err != nil {
		return nil, err
	}

	fhirClient, err := c.fhirClientFn(ctx, tenantURA)
	if err != nil {
		return nil, err
	}

	var result fhir.List
	err = fhirClient.CreateWithContext(ctx, tokenizedList, &result, fhirclient.AtPath("List"))
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to register List at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return &result, nil
}

// ReadList reads a single `List` resource by its NVI id.
func (c Component) ReadList(ctx context.Context, tenantURA string, id string) (*fhir.List, error) {
	fhirClient, err := c.fhirClientFn(ctx, tenantURA)
	if err != nil {
		return nil, err
	}

	var result fhir.List
	err = fhirClient.ReadWithContext(ctx, "List/"+id, &result)
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to read List at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return &result, nil
}

// DeleteListByID deletes a single `List` resource by its NVI id.
func (c Component) DeleteListByID(ctx context.Context, tenantURA string, id string) error {
	fhirClient, err := c.fhirClientFn(ctx, tenantURA)
	if err != nil {
		return err
	}

	if err := fhirClient.DeleteWithContext(ctx, "List/"+id); err != nil {
		return &fhirapi.Error{
			Message:   "Failed to delete List at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return nil
}

// DeleteListByParams deletes every `List` resource matching the given search parameters (at
// least one of patient:identifier, subject:identifier or source:identifier is required).
func (c Component) DeleteListByParams(ctx context.Context, tenantURA string, params url.Values) error {
	deleteParams, err := c.tokenizeSearchParams(ctx, params, tenantURA)
	if err != nil {
		return err
	}

	fhirClient, err := c.fhirClientFn(ctx, tenantURA)
	if err != nil {
		return err
	}

	if err := fhirClient.DeleteWithContext(ctx, "List?"+deleteParams.Encode()); err != nil {
		return &fhirapi.Error{
			Message:   "Failed to delete List at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return nil
}

// SearchList searches for `List` resources matching the given search parameters (at least one
// of patient:identifier, subject:identifier or source:identifier is required).
func (c Component) SearchList(ctx context.Context, tenantURA string, params url.Values) (*fhir.Bundle, error) {
	searchParams, err := c.tokenizeSearchParams(ctx, params, tenantURA)
	if err != nil {
		return nil, err
	}

	fhirClient, err := c.fhirClientFn(ctx, tenantURA)
	if err != nil {
		return nil, err
	}

	var searchSet fhir.Bundle
	err = fhirClient.SearchWithContext(ctx, "List", searchParams, &searchSet)
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to search for List resources at NVI",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}

	if hasNextLink(&searchSet) {
		// Otherwise must paginate, not supported for now.
		return nil, &fhirapi.Error{
			Message:   "NVI returned more results than can be handled. Please refine your search, or increase _count.",
			IssueType: fhir.IssueTypeTooCostly,
		}
	}

	return &searchSet, nil
}

// tokenizeSearchParams validates that at least one patient/subject/source identifier is
// present (to prevent querying/deleting by empty values), then converts BSN values to NVI
// transport tokens, mapping patient:identifier to subject:identifier (NVI's native parameter
// name) along the way. Shared by SearchList and DeleteListByParams, which apply the exact same
// parameter rules.
func (c Component) tokenizeSearchParams(ctx context.Context, params url.Values, tenantURA string) (url.Values, error) {
	hasIdentifier := false
	for key := range params {
		if key == "patient:identifier" ||
			key == "subject:identifier" ||
			key == "source:identifier" {
			hasIdentifier = true
			break
		}
	}
	if !hasIdentifier {
		return nil, fhirapi.BadRequestError("at least one of patient:identifier, subject:identifier or source:identifier is required", nil)
	}

	tokenizedParams := url.Values{}
	for key, values := range params {
		newValues := append([]string{}, values...)
		nviKey := key
		if key == "patient:identifier" {
			nviKey = "subject:identifier"
		}
		if key == "patient:identifier" ||
			key == "subject:identifier" ||
			strings.HasPrefix(key, coding.BSNNamingSystem) {
			for i, value := range values {
				newValue, err := c.tokenizeFHIRSearchToken(ctx, value, tenantURA, c.audience)
				if err != nil {
					return nil, err
				}
				newValues[i] = newValue
			}
		}
		tokenizedParams[nviKey] = append(tokenizedParams[nviKey], newValues...)
	}
	return tokenizedParams, nil
}

func (c Component) tokenizeListIdentifiers(ctx context.Context, resource fhir.List, localOrganizationURA string, audience string) (*fhir.List, error) {
	if err := c.reconcileCustodianExtension(&resource, localOrganizationURA); err != nil {
		return nil, err
	}
	if resource.Subject == nil || resource.Subject.Identifier == nil {
		return &resource, nil
	}
	tokenizedIdentifier, err := c.identifierToToken(ctx, *resource.Subject.Identifier, localOrganizationURA, audience)
	if err != nil {
		return nil, err
	}
	resource.Subject.Identifier = tokenizedIdentifier
	return &resource, nil
}

// reconcileCustodianExtension ensures the custodian extension in the List matches the tenant URA from the request header.
// If the extension is absent, it is added. If it is present but has a different URA, an error is returned.
func (c Component) reconcileCustodianExtension(resource *fhir.List, localOrganizationURA string) error {
	for _, ext := range resource.Extension {
		if ext.Url != coding.NVICustodianExtensionURL {
			continue
		}
		extValue := ""
		if ext.ValueReference != nil && ext.ValueReference.Identifier != nil && ext.ValueReference.Identifier.Value != nil {
			extValue = *ext.ValueReference.Identifier.Value
		}
		if extValue != localOrganizationURA {
			return fhirapi.BadRequestError(
				fmt.Sprintf("custodian extension URA (%s) does not match tenant ID header (%s)", extValue, localOrganizationURA),
				nil,
			)
		}
		return nil
	}
	// Extension absent — add it.
	resource.Extension = append(resource.Extension, fhir.Extension{
		Url: coding.NVICustodianExtensionURL,
		ValueReference: &fhir.Reference{
			Identifier: &fhir.Identifier{
				System: to.Ptr(coding.URANamingSystem),
				Value:  to.Ptr(localOrganizationURA),
			},
		},
	})
	return nil
}

// tokenizeFHIRSearchToken converts a FHIR search token  (<system>|<value>) to a BSN transport token value.
func (c Component) tokenizeFHIRSearchToken(ctx context.Context, searchToken string, localOrganizationURA string, audience string) (string, error) {
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
	tokenizedIdentifier, err := c.identifierToToken(ctx, identifier, localOrganizationURA, audience)
	if err != nil {
		return "", err
	}
	return *tokenizedIdentifier.System + "|" + *tokenizedIdentifier.Value, nil
}

func (c Component) identifierToToken(ctx context.Context, identifier fhir.Identifier, localOrganizationURA string, audience string) (*fhir.Identifier, error) {
	result, err := c.pseudonymizer.IdentifierToToken(ctx, identifier, localOrganizationURA, audience, "nationale-verwijsindex")
	if err != nil {
		return nil, &fhirapi.Error{
			Message:   "Failed to pseudonymize BSN identifier",
			Cause:     err,
			IssueType: fhir.IssueTypeTransient,
		}
	}
	return result, nil
}

// searchParamValues builds the url.Values SearchList/DeleteListByParams expect from the
// generated, individually-bound query parameters. code and count are nil for DeleteNVIList's
// params, which don't declare them.
func searchParamValues(patientIdentifier, subjectIdentifier, sourceIdentifier, code *string, count *int) url.Values {
	values := url.Values{}
	set := func(key string, value *string) {
		if value != nil {
			values.Set(key, *value)
		}
	}
	set("patient:identifier", patientIdentifier)
	set("subject:identifier", subjectIdentifier)
	set("source:identifier", sourceIdentifier)
	set("code", code)
	if count != nil {
		values.Set("_count", strconv.Itoa(*count))
	}
	return values
}

func (c Component) RegisterNVIListBundle(ctx context.Context, request api.RegisterNVIListBundleRequestObject) (api.RegisterNVIListBundleResponseObject, error) {
	result, err := c.RegisterBundle(ctx, request.Params.XTenantID.URA(), *request.Body)
	if err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.RegisterNVIListBundle200ApplicationFhirPlusJSONResponse(*result), nil
}

func (c Component) RegisterNVIList(ctx context.Context, request api.RegisterNVIListRequestObject) (api.RegisterNVIListResponseObject, error) {
	result, err := c.registerList(ctx, request.Params.XTenantID.URA(), *request.Body)
	if err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.RegisterNVIList200ApplicationFhirPlusJSONResponse(*result), nil
}

func (c Component) GetNVIList(ctx context.Context, request api.GetNVIListRequestObject) (api.GetNVIListResponseObject, error) {
	result, err := c.ReadList(ctx, request.Params.XTenantID.URA(), request.Id)
	if err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.GetNVIList200ApplicationFhirPlusJSONResponse(*result), nil
}

func (c Component) DeleteNVIList(ctx context.Context, request api.DeleteNVIListRequestObject) (api.DeleteNVIListResponseObject, error) {
	if err := c.DeleteListByID(ctx, request.Params.XTenantID.URA(), request.Id); err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.DeleteNVIList204Response{}, nil
}

func (c Component) DeleteNVIListsByParams(ctx context.Context, request api.DeleteNVIListsByParamsRequestObject) (api.DeleteNVIListsByParamsResponseObject, error) {
	params := searchParamValues(request.Params.PatientIdentifier, request.Params.SubjectIdentifier, request.Params.SourceIdentifier, nil, nil)
	if err := c.DeleteListByParams(ctx, request.Params.XTenantID.URA(), params); err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.DeleteNVIListsByParams204Response{}, nil
}

func (c Component) SearchNVILists(ctx context.Context, request api.SearchNVIListsRequestObject) (api.SearchNVIListsResponseObject, error) {
	params := searchParamValues(request.Params.PatientIdentifier, request.Params.SubjectIdentifier, request.Params.SourceIdentifier, request.Params.Code, request.Params.UnderscoreCount)
	result, err := c.SearchList(ctx, request.Params.XTenantID.URA(), params)
	if err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.SearchNVILists200ApplicationFhirPlusJSONResponse(*result), nil
}

func (c Component) SearchNVIListsForm(ctx context.Context, request api.SearchNVIListsFormRequestObject) (api.SearchNVIListsFormResponseObject, error) {
	var body api.SearchNVIListsFormFormdataRequestBody
	if request.Body != nil {
		body = *request.Body
	}
	params := searchParamValues(body.PatientIdentifier, body.SubjectIdentifier, body.SourceIdentifier, body.Code, body.UnderscoreCount)
	result, err := c.SearchList(ctx, request.Params.XTenantID.URA(), params)
	if err != nil {
		return api.NewOperationOutcomeResponse(ctx, err), nil
	}
	return api.SearchNVIListsForm200ApplicationFhirPlusJSONResponse(*result), nil
}

func (c Component) Start() error {
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	return nil
}

type loggingTransport struct {
	next http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	slog.DebugContext(req.Context(), "NVI request", "method", req.Method, "url", req.URL.String())
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		slog.DebugContext(req.Context(), "NVI request failed", "method", req.Method, "url", req.URL.String(), "error", err)
		return nil, err
	}
	slog.DebugContext(req.Context(), "NVI response", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode)
	return resp, nil
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
