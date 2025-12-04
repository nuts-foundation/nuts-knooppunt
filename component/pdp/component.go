package pdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
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

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	// Nothing to do
	return nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("POST /pdp", http.HandlerFunc(c.HandleMainPolicy))
	internalMux.HandleFunc("POST /pdp/v1/data/{package}/{rule}", http.HandlerFunc(c.HandlePolicy))
}

func (c Component) HandleMainPolicy(w http.ResponseWriter, r *http.Request) {
	var reqBody MainPolicyRequest
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	input := reqBody.Input
	if err != nil {
		http.Error(w, "unable to parse request body", http.StatusBadRequest)
		return
	}

	// Step 1: Providing a scope is required for every PDP request
	// FUTURE: We are considering allowing more than one scope
	if input.Scope == "" {
		res := PolicyResult{
			Allow: false,
			Reasons: []ResultReason{
				{
					Code:        "missing_required_value",
					Description: "missing required value, no scope defined",
				},
			},
		}
		writeResp(r.Context(), w, res)
		return
	}

	// Step 2: Check the request adheres to the capability statement for this scope
	res := evalCapabilityPolicy(r.Context(), input)
	if !res.Allow {
		writeResp(r.Context(), w, res)
		return
	}

	// Step 3: Check if we are authorized to see the underlying data
	// FUTURE: We want to use OPA policies here ...
	// ... but for now we only have two example scopes hardcoded.
	switch input.Scope {
	case "mcsd_update":
		validTypes := []fhir.ResourceType{
			fhir.ResourceTypeOrganization,
			fhir.ResourceTypeLocation,
			fhir.ResourceTypeHealthcareService,
			fhir.ResourceTypeEndpoint,
			fhir.ResourceTypePractitionerRole,
			fhir.ResourceTypeOrganizationAffiliation,
			fhir.ResourceTypePractitioner,
		}
		if !slices.Contains(validTypes, input.ResourceType) {
			writeResp(r.Context(), w, Deny(ResultReason{
				Code:        "not_allowed",
				Description: "not allowed to request this resources during update",
			}))
		}
		writeResp(r.Context(), w, Allow())
	case "patient_example":
		writeResp(r.Context(), w, EvalMitzPolicy(c, r.Context(), input))
	default:
		writeResp(r.Context(), w, Deny(
			ResultReason{
				Code:        "not_implemented",
				Description: fmt.Sprintf("scope %s not implemeted", input.Scope),
			},
		))
	}
}

func writeResp(ctx context.Context, w http.ResponseWriter, result PolicyResult) {
	resp := MainPolicyResponse{
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
