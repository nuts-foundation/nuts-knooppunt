package pdp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
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
	Mitz   *mitz.Component
}

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
}

type MainPolicyInput struct {
	Method                           string   `json:"method"`
	Path                             []string `json:"path"`
	PatientBSN                       string   `json:"patient_bsn"`
	RequestingUziRoleCode            string   `json:"requesting_uzi_role_code"`
	RequestingPractitionerIdentifier string   `json:"requesting_practitioner_identifier"`
	RequestingOrganizationUra        string   `json:"requesting_organization_ura"`
	RequestingFacilityType           string   `json:"requesting_facility_type"`
	DataHolderOrganizationUra        string   `json:"data_holder_organization_ura"`
	DataHolderFacilityType           string   `json:"data_holder_facility_type"`
	PurposeOfUse                     string   `json:"purpose_of_use"`
}

type MainPolicyRequest struct {
	Input MainPolicyInput `json:"input"`
}

type MainPolicyResponse struct {
	Result MainPolicyResult `json:"result"`
}

type MainPolicyResult struct {
	Allow bool `json:"allow"`
}

func (c Component) HandleMainPolicy(w http.ResponseWriter, r *http.Request) {
	var reqBody MainPolicyRequest
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	input := reqBody.Input
	if err != nil {
		http.Error(w, "unable to parse request body", http.StatusBadRequest)
		return
	}

	ok := validateInput(input)
	if !ok {
		http.Error(w, "input not valid", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	mitzComp := *c.Mitz
	consentReq := xacml.AuthzRequest{
		PatientBSN:             input.PatientBSN,
		HealthcareFacilityType: input.DataHolderFacilityType,
		AuthorInstitutionID:    input.DataHolderOrganizationUra,
		EventCode:              "GGC002",
		SubjectRole:            input.RequestingUziRoleCode,
		ProviderID:             input.RequestingUziRoleCode,
		ProviderInstitutionID:  input.RequestingOrganizationUra,
		ConsultingFacilityType: input.RequestingFacilityType,
		PurposeOfUse:           "TREAT",
	}
	consentResp, err := mitzComp.CheckConsent(ctx, consentReq)
	if err != nil {
		http.Error(w, "could not complete the consent check", http.StatusInternalServerError)
	}

	allow := false
	switch consentResp.Decision {
	case xacml.DecisionPermit:

		allow = true
	case xacml.DecisionDeny:

		allow = false
	default:
		allow = false
	}

	resp := MainPolicyResponse{
		Result: MainPolicyResult{
			Allow: allow,
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode json output", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func validateInput(input MainPolicyInput) bool {
	requiredValues := []string{
		input.Method,
		input.PatientBSN,
		input.RequestingUziRoleCode,
		input.RequestingPractitionerIdentifier,
		input.RequestingOrganizationUra,
		input.RequestingFacilityType,
		input.DataHolderOrganizationUra,
		input.DataHolderFacilityType,
		input.PurposeOfUse,
	}
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
