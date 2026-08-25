package pdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/policies"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"golang.org/x/exp/maps"
)

func DefaultConfig() Config {
	return Config{
		Enabled: true,
		PIP: PIPConfig{
			URL: "",
		},
	}
}

var _ component.Lifecycle = (*Component)(nil)

// New creates an instance of the pdp component, which provides a simple policy decision endpoint.
func New(config Config, consentChecker mitz.ConsentChecker) (*Component, error) {
	comp := &Component{
		Config:           config,
		consentChecker:   consentChecker,
		opaBundleBaseURL: "http://localhost:8081/pdp/bundles/",
	}

	if config.PIP.URL != "" {
		url, err := url.Parse(config.PIP.URL)
		if err != nil {
			return &Component{}, err
		}
		pipClient := fhirclient.New(url, tracing.NewHTTPClient(), &fhirclient.Config{
			UsePostSearch: false,
		})
		comp.pipClient = pipClient
	} else {
		slog.Warn("PIP address not configured, authorization limited to self contained policies")
	}

	return comp, nil
}

func (c *Component) Start() error {
	bundles, err := policies.Bundles(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load policy bundles: %w", err)
	}
	opaService, err := createOPAService(context.Background(), c.opaBundleBaseURL, bundles)
	if err != nil {
		return fmt.Errorf("failed to initialize Open Policy Agent service: %w", err)
	}
	c.opaService = opaService
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.opaService.Stop(ctx)
	return nil
}

// RegisterHttpHandlers registers no routes: every PDP operation is served through the generated
// OpenAPI strict server, wired up in strictAPIServer.RegisterHttpHandlers in package cmd.
func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
}

func (c *Component) HandleMainPolicy(w http.ResponseWriter, r *http.Request) {
	var reqBody APIRequest
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	if err != nil {
		writeResponseWithCode(r.Context(), w, APIResponse{
			Error:    "unable to parse request body: " + err.Error(),
			Policies: map[string]PolicyResult{},
		}, http.StatusBadRequest)
		return
	}
	response, statusCode := c.Evaluate(r.Context(), reqBody)
	writeResponseWithCode(r.Context(), w, response, statusCode)
}

// Evaluate runs the core PDP evaluation: given an already-decoded request, it returns the
// authorization decision and the HTTP status code it should be served with. Split out from
// HandleMainPolicy so it can be called directly (e.g. by the generated strict API server)
// without going through the HTTP layer.
func (c *Component) Evaluate(ctx context.Context, reqBody APIRequest) (APIResponse, int) {
	input := reqBody.Input

	scopes := strings.Fields(input.Subject.Scope)

	// deduplicate and normalize policies
	policySet := make(map[string]struct{})
	for _, policyName := range scopes {
		policyName = policies.NormalizePolicyName(policyName)
		policySet[policyName] = struct{}{}
	}
	policyNames := maps.Keys(policySet)
	slices.Sort(policyNames)

	// Step 1: Providing a policy is required for every PDP request. We can short-circuit here, no need to process the request.
	// The `system` bundle hosts OPA infrastructure rules (e.g. decision-log masking) and is not directly invokable.
	for _, policyName := range policyNames {
		if strings.HasPrefix(policyName, "test_") || policyName == "system" {
			return APIResponse{
				Error:    fmt.Sprintf("policy not allowed: %s", policyName),
				Policies: map[string]PolicyResult{},
			}, http.StatusBadRequest
		}
	}
	if len(policyNames) == 0 {
		return APIResponse{
			Error:    "missing required value, no policy defined",
			Policies: map[string]PolicyResult{},
		}, http.StatusOK
	}

	response := APIResponse{
		Policies: make(map[string]PolicyResult),
	}

	// Step 2: Parse the PDP input and translate to the policy input
	policyInputTemplate, err := NewPolicyInput(reqBody)
	if err != nil {
		// Invalid request
		return APIResponse{
			Error: "invalid request: " + err.Error(),
		}, http.StatusOK
	}

	// Step 3: Enrich the policy input with data gathered from the policy information point (if available)
	policyInputTemplate, resultReasonsPIP := c.enrichPolicyInputWithPIP(ctx, policyInputTemplate)
	// Step 4: Check consent at Mitz
	policyInputTemplate, resultReasonsMitz := c.enrichPolicyInputWithMitz(ctx, policyInputTemplate)
	resultReasons := slices.Concat(resultReasonsPIP, resultReasonsMitz)

	// Evaluate all policies
	for _, policyName := range policyNames {
		policyInput := policyInputTemplate.Copy()
		policyResult := PolicyResult{
			// Include all previously collected result reasons.
			// Make sure we operate on a copy of resultReasons, to avoid duplication to to append() sem
			Reasons: append([]ResultReason(nil), resultReasons...),
		}

		// Check if the policy exists
		{
			policyExists, err := c.policyExists(ctx, policyName)
			if err != nil {
				slog.ErrorContext(ctx, "failed to check if policy exists", logging.Error(err), slog.String("policy", policyName))
				policyResult.Reasons = append(policyResult.Reasons, ResultReason{
					Code:        TypeResultCodeInternalError,
					Description: fmt.Sprintf("failed to check if policy exists: %v", err),
				})
				response.Policies[policyName] = policyResult
				continue
			}
			if !policyExists {
				policyResult.Reasons = append(policyResult.Reasons, ResultReason{
					Code:        TypeResultCodeNotImplemented,
					Description: fmt.Sprintf("unknown policy: %s", policyName),
				})
				response.Policies[policyName] = policyResult
				continue
			}
		}

		// Step 5: Check FHIR Capability Statement
		{
			var fhirCapStatCheckResultReasons []ResultReason
			policyInput, fhirCapStatCheckResultReasons = enrichPolicyInputWithCapabilityStatement(ctx, policyInput, policyName)
			policyResult.Reasons = append(policyResult.Reasons, fhirCapStatCheckResultReasons...)
		}

		// Step 6: Evaluate using Open Policy Agent
		{
			regoPolicyResult, err := c.evalRegoPolicy(ctx, policyName, policyInput)
			if err != nil {
				slog.ErrorContext(ctx, "failed to evaluate rego policy", logging.Error(err), slog.String("policy", policyName))
				policyResult.Reasons = append(policyResult.Reasons, ResultReason{
					Code:        TypeResultCodeInternalError,
					Description: "failed to evaluate rego policy: " + err.Error(),
				})
			} else {
				policyResult.Reasons = append(policyResult.Reasons, regoPolicyResult.Reasons...)
				policyResult.Allow = regoPolicyResult.Allow
			}
		}
		response.Policies[policyName] = policyResult
		if policyResult.Allow {
			// Found policy that allows access, no need to evaluate other policies
			response.Allow = true
			break
		}
	}

	return response, http.StatusOK
}

func (c *Component) EvaluateAuthorization(ctx context.Context, request api.EvaluateAuthorizationRequestObject) (api.EvaluateAuthorizationResponseObject, error) {
	apiRequest, err := to.JSONConvert[APIRequest](request.Body)
	if err != nil {
		return nil, err
	}
	result, statusCode := c.Evaluate(ctx, apiRequest)
	apiResponse, err := to.JSONConvert[api.PDPAuthzResponse](result)
	if err != nil {
		return nil, err
	}
	switch statusCode {
	case http.StatusOK:
		return api.EvaluateAuthorization200JSONResponse(apiResponse), nil
	case http.StatusBadRequest:
		return api.EvaluateAuthorization400JSONResponse(apiResponse), nil
	default:
		return nil, fmt.Errorf("unexpected status code from PDP evaluation: %d", statusCode)
	}
}

// EvaluateDefaultAuthorization backs POST /pdp, a shorter path to the same, single policy that
// POST /pdp/v1/data/knooppunt/authz addresses via the Open Policy Agent convention.
func (c *Component) EvaluateDefaultAuthorization(ctx context.Context, request api.EvaluateDefaultAuthorizationRequestObject) (api.EvaluateDefaultAuthorizationResponseObject, error) {
	apiRequest, err := to.JSONConvert[APIRequest](request.Body)
	if err != nil {
		return nil, err
	}
	result, statusCode := c.Evaluate(ctx, apiRequest)
	apiResponse, err := to.JSONConvert[api.PDPAuthzResponse](result)
	if err != nil {
		return nil, err
	}
	switch statusCode {
	case http.StatusOK:
		return api.EvaluateDefaultAuthorization200JSONResponse(apiResponse), nil
	case http.StatusBadRequest:
		return api.EvaluateDefaultAuthorization400JSONResponse(apiResponse), nil
	default:
		return nil, fmt.Errorf("unexpected status code from PDP evaluation: %d", statusCode)
	}
}

func writeResponseWithCode(ctx context.Context, w http.ResponseWriter, response any, statusCode int) {
	b, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to encode json output", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, err = w.Write(b)
	if err != nil {
		slog.ErrorContext(ctx, "failed to write response to ResponseWriter", logging.Error(err))
	}
}

// bundleNames returns the names of the loaded Open Policy Agent bundles.
func (c *Component) bundleNames(ctx context.Context) ([]string, error) {
	bundles, err := policies.Bundles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve bundles: %w", err)
	}
	return maps.Keys(bundles), nil
}

// bundle returns the raw OPA bundle (gzipped tar) for the given policy, trimming a trailing
// ".tar.gz" suffix if present. found reports whether a bundle with that name is loaded.
func (c *Component) bundle(ctx context.Context, policyName string) (data []byte, found bool, err error) {
	policyName = strings.TrimSuffix(policyName, ".tar.gz")
	bundles, err := policies.Bundles(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to retrieve bundles: %w", err)
	}
	data, found = bundles[policyName]
	return data, found, nil
}

func (c *Component) ListAuthorizationPolicyBundles(ctx context.Context, _ api.ListAuthorizationPolicyBundlesRequestObject) (api.ListAuthorizationPolicyBundlesResponseObject, error) {
	names, err := c.bundleNames(ctx)
	if err != nil {
		// No error response is declared for this internal-use endpoint; falls through to the
		// framework's generic 500 plain-text handler, same as the original http.Error call.
		return nil, err
	}
	return api.ListAuthorizationPolicyBundles200JSONResponse(names), nil
}

func (c *Component) GetAuthorizationPolicyBundle(ctx context.Context, request api.GetAuthorizationPolicyBundleRequestObject) (api.GetAuthorizationPolicyBundleResponseObject, error) {
	data, found, err := c.bundle(ctx, request.PolicyName)
	if err != nil {
		return nil, err
	}
	if !found {
		return api.GetAuthorizationPolicyBundle404Response{}, nil
	}
	return rawBundleResponse{
		contentType:        "application/gzip",
		contentDisposition: fmt.Sprintf("attachment; filename=%s.tar.gz", request.PolicyName),
		body:               data,
	}, nil
}

// rawBundleResponse implements api.GetAuthorizationPolicyBundleResponseObject by hand: the OpenAPI
// spec doesn't declare a content schema for this response (it's an internal-use, binary
// download), so oapi-codegen has nothing to generate a typed body from. The response object is
// just an interface (VisitGetAuthorizationPolicyBundleResponse(w) error), so a hand-written
// implementation slots in the same way a generated one would.
type rawBundleResponse struct {
	contentType        string
	contentDisposition string
	body               []byte
}

func (r rawBundleResponse) VisitGetAuthorizationPolicyBundleResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", r.contentType)
	w.Header().Set("Content-Disposition", r.contentDisposition)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(r.body)
	return err
}

func (c *Component) policyExists(ctx context.Context, policy string) (bool, error) {
	bundles, err := policies.Bundles(ctx)
	if err != nil {
		return false, err
	}
	_, found := bundles[policy]
	return found, nil
}
