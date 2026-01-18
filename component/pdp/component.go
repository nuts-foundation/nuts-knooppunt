package pdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
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
func New(config Config, mitzcomp *mitz.Component) (*Component, error) {
	comp := &Component{
		Config: config,
		Mitz:   mitzcomp,
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

	// Step 3: Enrich the policy input with data gathered from the policy information point (if available)
	policyInput = PipPolicyInput(c, policyInput)

	// Step 4: Check the request adheres to the capability statement for this scope
	res := evalCapabilityPolicy(r.Context(), policyInput)
	if !res.Allow {
		writeResp(r.Context(), w, res)
		return
	}

	// Step 5: Check the request adheres to the capability statement for this scope
	policyInput, mitzPolicyResult := EvalMitzPolicy(c, r.Context(), policyInput)

	// Step 6: Check if we are authorized to see the underlying data
	// FUTURE: We want to use OPA policies here ...
	// ... but for now we only have same example scopes hardcoded.
	// This section is very much work in progress
	switch scope {
	case "mcsd_update":
		// Dummy should be replaced with the actual OPA policy
		writeResp(r.Context(), w, Allow())
	case "mcsd_query":
		// Dummy should be replaced with the actual OPA policy
		writeResp(r.Context(), w, Allow())
	case "bgz_patient":
		// Dummy should be replaced with the actual OPA policy
		writeResp(r.Context(), w, mitzPolicyResult)
	case "bgz_professional":
		// Dummy should be replaced with the actual OPA policy
		writeResp(r.Context(), w, mitzPolicyResult)
	default:
		writeResp(r.Context(), w, Deny(
			ResultReason{
				Code:        TypeResultCodeNotImplemented,
				Description: fmt.Sprintf("scope %s not implemeted", scope),
			},
		))
	}
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
