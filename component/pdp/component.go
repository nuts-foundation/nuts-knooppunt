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
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/policies"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
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
	opaService, err := createOPAService(context.Background(), c.opaBundleBaseURL)
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

func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("POST /pdp", c.HandleMainPolicy)
	internalMux.HandleFunc("POST /pdp/v1/data/{package}/{rule}", c.HandlePolicy)
	// The following endpoint lists the available OPA policy bundles.
	// It's not used by Open Policy Agent, but can be useful for debugging and operational purposes.
	internalMux.HandleFunc("GET /pdp/bundles", c.HandleListBundles)
	// The following endpoint serves the OPA policy bundle for a specific scope.
	// It's used by Open Policy Agent on startup to load the policy bundles.
	internalMux.HandleFunc("GET /pdp/bundles/{policyName}", c.HandleGetBundle)
}

func (c *Component) HandleMainPolicy(w http.ResponseWriter, r *http.Request) {
	var reqBody PDPRequest
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	input := reqBody.Input
	if err != nil {
		writeResponseWithCode(r.Context(), w, PDPResponse{
			Result: PolicyResult{
				Reasons: []ResultReason{
					{
						Code:        TypeResultCodeUnexpectedInput,
						Description: "unable to parse request body: " + err.Error(),
					},
				},
			},
		}, http.StatusBadRequest)
		return
	}

	policyNames := input.Subject.Properties.ClientQualifications
	// deduplicate policies, just in case
	policySet := make(map[string]struct{})
	for _, policy := range policyNames {
		policySet[policy] = struct{}{}
	}
	policyNames = maps.Keys(policySet)
	slices.Sort(policyNames)

	// Step 1: Providing a policy is required for every PDP request. We can short-circuit here, no need to process the request.
	if len(policyNames) == 0 {
		writeResponse(r.Context(), w, PDPResponse{
			Result: PolicyResult{
				Reasons: []ResultReason{
					{
						Code:        TypeResultCodeMissingRequiredValue,
						Description: "missing required value, no policy defined",
					},
				},
			},
		})
		return
	}

	response := PDPResponse{
		Policies: make(map[string]PolicyResult),
	}

	// Step 2: Parse the PDP input and translate to the policy input
	policyInputPtr, resultReasons := NewPolicyInput(reqBody)
	if policyInputPtr == nil {
		// Invalid request
		response.Result.Reasons = append(response.Result.Reasons, resultReasons...)
		writeResponse(r.Context(), w, response)
		return
	}
	policyInput := *policyInputPtr

	// Step 2: Enrich the policy input with data gathered from the policy information point (if available)
	policyInput, resultReasons = c.enrichPolicyInputWithPIP(r.Context(), policyInput)
	response.Result.Reasons = append(response.Result.Reasons, resultReasons...)

	// Step 3: Check consent at Mitz
	policyInput, resultReasons = c.enrichPolicyInputWithMitz(r.Context(), policyInput)
	response.Result.Reasons = append(response.Result.Reasons, resultReasons...)

	// Evaluate all known policies
	for _, policyName := range policyNames {
		thisPolicyInput := policyInput.Copy()

		// OPA doesn't support dashes in package and rule names, so we replace them with underscores.
		policyName = strings.ReplaceAll(policyName, "-", "_")

		// Check if the policy exists
		policyExists, err := c.policyExists(r.Context(), policyName)
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to check if policy exists", logging.Error(err), slog.String("policy", policyName))
			response.Result.Reasons = append(response.Result.Reasons, ResultReason{
				Code:        TypeResultCodeInternalError,
				Description: fmt.Sprintf("failed to check if policy exists: %v", err),
			})
			continue
		}
		if !policyExists {
			response.Result.Reasons = append(response.Result.Reasons, ResultReason{
				Code:        TypeResultCodeNotImplemented,
				Description: fmt.Sprintf("unknown policy: %s", policyName),
			})
			continue
		}

		var policyResultReasons []ResultReason

		// Step 4: Check FHIR Capability Statement
		thisPolicyInput, policyResultReasons = enrichPolicyInputWithCapabilityStatement(r.Context(), thisPolicyInput, policyName)

		// Step 5: Evaluate using Open Policy Agent
		policyResult, err := c.evalRegoPolicy(r.Context(), policyName, thisPolicyInput)
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to evaluate rego policy", logging.Error(err), slog.String("policy", policyName))
			policyResult = &PolicyResult{
				Reasons: []ResultReason{
					{
						Code:        TypeResultCodeNotImplemented,
						Description: "failed to evaluate rego policy: " + err.Error(),
					},
				},
			}
		}
		policyResult.Reasons = append(policyResultReasons, policyResult.Reasons...)
		response.Policies[policyName] = *policyResult
		if policyResult.Allow {
			// Found policy that allows access, no need to evaluate other policies
			response.Result.Allow = true
			break
		}
	}

	writeResponse(r.Context(), w, response)
}

func writeResponseWithCode(ctx context.Context, w http.ResponseWriter, response PDPResponse, statusCode int) {
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

func writeResponse(ctx context.Context, w http.ResponseWriter, result PDPResponse) {
	writeResponseWithCode(ctx, w, result, http.StatusOK)
}

func (c *Component) HandlePolicy(w http.ResponseWriter, r *http.Request) {
	pack := r.PathValue("package")
	if pack != "knooppunt" {
		http.Error(w, "invalid package", http.StatusBadRequest)
		return
	}

	policy := r.PathValue("rule")
	switch policy {
	case "authz":
		c.HandleMainPolicy(w, r)
	default:
		http.Error(w, fmt.Sprintf("unknown rule %s", policy), http.StatusBadRequest)
	}
}

// HandleListBundles returns a list of available OPA policy bundles
func (c *Component) HandleListBundles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bundles, err := policies.Bundles(r.Context())
	if err != nil {
		http.Error(w, "failed to retrieve bundles", http.StatusInternalServerError)
		slog.ErrorContext(r.Context(), "Failed to retrieve bundles", logging.Error(err))
		return
	}
	if err := json.NewEncoder(w).Encode(maps.Keys(bundles)); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		slog.ErrorContext(r.Context(), "Failed to encode bundles list", logging.Error(err))
	}
}

// HandleGetBundle serves an OPA policy bundle for a specific scope
func (c *Component) HandleGetBundle(w http.ResponseWriter, r *http.Request) {
	policyName := r.PathValue("policyName")
	if policyName == "" {
		// Shouldn't happen, but still...
		http.Error(w, "policyName parameter is required", http.StatusBadRequest)
		return
	}
	policyName = strings.TrimSuffix(policyName, ".tar.gz")

	bundles, err := policies.Bundles(r.Context())
	if err != nil {
		http.Error(w, "failed to retrieve bundles", http.StatusInternalServerError)
		slog.ErrorContext(r.Context(), "Failed to retrieve bundles", logging.Error(err))
		return
	}
	bundleData, found := bundles[policyName]
	if !found {
		http.Error(w, fmt.Sprintf("bundle not found: %s", policyName), http.StatusNotFound)
		slog.WarnContext(r.Context(), "Bundle not found", slog.String("policyName", policyName))
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar.gz", policyName))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(bundleData); err != nil {
		slog.ErrorContext(r.Context(), "Failed to write bundle",
			slog.String("policyName", policyName),
			logging.Error(err))
	}
}

func (c *Component) policyExists(ctx context.Context, policy string) (bool, error) {
	bundles, err := policies.Bundles(ctx)
	if err != nil {
		return false, err
	}
	_, found := bundles[policy]
	return found, nil
}
