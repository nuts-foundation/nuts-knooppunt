package pdp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component"
)

type Config struct {
	Enabled bool
}

func DefaultConfig() Config {
	return Config{
		Enabled: true,
	}
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	Config Config
}

// New creates an instance of the pdp component, which provides a simple policy decision endpoint.
func New(config Config) (*Component, error) {
	return &Component{
		Config: config,
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
	internalMux.HandleFunc("POST /pdp", mainPolicyHandler)
}

type MainPolicyInput struct {
	Method       string   `json:"method"`
	Path         []string `json:"path"`
	SubjectType  string   `json:"subject_type"`
	SubjectId    string   `json:"subject_id"`
	SubjectRole  *string  `json:"subject_organization_id"`
	PurposeOfUse string   `json:"purpose_of_use"`
}

type MainPolicyResponse struct {
	Result MainPolicyResult `json:"result"`
}

type MainPolicyResult struct {
	Allow bool `json:"allow"`
}

func mainPolicyHandler(w http.ResponseWriter, r *http.Request) {
	var input MainPolicyInput
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "unable to parse request body", http.StatusBadRequest)
		return
	}

	ok := validateInput(input)
	if !ok {
		http.Error(w, "input not valid", http.StatusBadRequest)
		return
	}

	resp := MainPolicyResponse{
		Result: MainPolicyResult{
			Allow: false,
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func validateInput(input MainPolicyInput) bool {
	requiredValues := []string{input.SubjectId, input.SubjectType, input.PurposeOfUse}
	if slices.Contains(requiredValues, "") {
		return false
	}

	validMethods := []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPatch}
	if !slices.Contains(validMethods, input.Method) {
		return false
	}

	// Add more validations here once we agreed upon a contract

	return true
}
