package pdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/bundles"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"golang.org/x/exp/maps"
)

func DefaultConfig() Config {
	return Config{
		Enabled: true,
	}
}

var _ component.Lifecycle = (*Component)(nil)

// New creates an instance of the pdp component, which provides a simple policy decision endpoint.
func New(config Config, mitzcomp *mitz.Component) (*Component, error) {
	return &Component{
		Config: config,
		Mitz:   mitzcomp,
	}, nil
}

func (c *Component) Start() error {
	opaService, err := createOPAService(context.Background())
	if err != nil {
		return fmt.Errorf("failed to initialize opaService service: %w", err)
	}
	c.opaService = opaService
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	c.opaService.Stop(ctx)
	return nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("POST /pdp", c.HandleMainPolicy)
	internalMux.HandleFunc("POST /pdp/v1/data/{package}/{rule}", c.HandlePolicy)
	// Serve opaService policy bundles
	internalMux.HandleFunc("GET /pdp/bundles", c.HandleListBundles)
	internalMux.HandleFunc("GET /pdp/bundles/{policyName}", c.HandleGetBundle)
}

func (c Component) HandleMainPolicy(w http.ResponseWriter, r *http.Request) {
	var reqBody PDPRequest
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	input := reqBody.Input
	if err != nil {
		http.Error(w, "unable to parse request body", http.StatusBadRequest)
		return
	}

	qualifications := input.Subject.Properties.ClientQualifications

	// Step 1: Providing a scope is required for every PDP request
	if len(qualifications) == 0 {
		res := PolicyResult{
			Allow: false,
			Reasons: []ResultReason{
				{
					Code:        TypeResultCodeMissingRequiredValue,
					Description: "missing required value, no scope defined",
				},
			},
		}
		writeResp(r.Context(), w, res)
		return
	}

	if len(qualifications) > 1 {
		res := PolicyResult{
			Allow: false,
			Reasons: []ResultReason{
				{
					Code:        TypeResultCodeNotImplemented,
					Description: "providing multiple qualifications is not yet implemented",
				},
			},
		}
		writeResp(r.Context(), w, res)
		return
	}

	// TODO: Implement support for multiple scopes
	scope := qualifications[0]

	// Step 2: Parse the PDP input and translate to the policy input
	policyInput, policyResult := NewPolicyInput(reqBody)
	if !policyResult.Allow {
		writeResp(r.Context(), w, policyResult)
		return
	}

	// Step 3: Check the request adheres to the capability statement for this scope
	res := evalCapabilityPolicy(r.Context(), policyInput)
	if !res.Allow {
		writeResp(r.Context(), w, res)
		return
	}

	// Step 4: Evaluate using Open Policy Agent
	regoPolicyResult, err := c.evalRegoPolicy(r.Context(), scope, policyInput)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to evaluate rego policy", logging.Error(err), slog.String("policy", scope))
	}
	writeResp(r.Context(), w, *regoPolicyResult)
}

func writeResp(ctx context.Context, w http.ResponseWriter, result PolicyResult) {
	resp := PDPResponse{
		Result: result,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode json output", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(b)
	if err != nil {
		slog.ErrorContext(ctx, "failed to write response to ResponseWriter", logging.Error(err))
	}
}

func (c Component) HandlePolicy(w http.ResponseWriter, r *http.Request) {
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

// HandleListBundles returns a list of available OPAService policy bundles
func (c Component) HandleListBundles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(maps.Keys(bundles.BundleMap)); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		slog.ErrorContext(r.Context(), "Failed to encode bundles list", logging.Error(err))
	}
}

// HandleGetBundle serves an OPAService policy bundle for a specific scope
func (c Component) HandleGetBundle(w http.ResponseWriter, r *http.Request) {
	policyName := r.PathValue("policyName")
	if policyName == "" {
		// Shouldn't happen, but still...
		http.Error(w, "policyName parameter is required", http.StatusBadRequest)
		return
	}
	policyName = strings.TrimSuffix(policyName, ".tar.gz")

	bundleData, found := bundles.BundleMap[policyName]
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
