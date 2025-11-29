package pdp

import (
	"context"
	"encoding/json"
	"fmt"
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
	internalMux.HandleFunc("POST /pdp/v1/data/{package}/{rule}", http.HandlerFunc(c.HandlePolicy))
}

type MainPolicyInput struct {
	DataHolderFacilityType           string   `json:"data_holder_facility_type"`
	DataHolderOrganizationUra        string   `json:"data_holder_organization_ura"`
	Method                           string   `json:"method"`
	Path                             []string `json:"path"`
	PatientBSN                       string   `json:"patient_bsn"`
	PurposeOfUse                     string   `json:"purpose_of_use"`
	RequestingFacilityType           string   `json:"requesting_facility_type"`
	RequestingOrganizationUra        string   `json:"requesting_organization_ura"`
	RequestingPractitionerIdentifier string   `json:"requesting_practitioner_identifier"`
	RequestingUziRoleCode            string   `json:"requesting_uzi_role_code"`
	ResourceId                       string   `json:"resource_id"`
	ResourceType                     string   `json:"resource_type"`
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

	ctx := r.Context()
	mitzComp := *c.Mitz
	consentReq := xacmlFromInput(input)
	consentResp, err := mitzComp.CheckConsent(ctx, consentReq)
	if err != nil {
		http.Error(w, "could not complete the consent check", http.StatusInternalServerError)
		return
	}

	allow := false
	if consentResp.Decision == xacml.DecisionPermit {
		allow = true
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

func xacmlFromInput(input MainPolicyInput) xacml.AuthzRequest {
	var purpose string
	switch input.PurposeOfUse {
	case "treatment":
		purpose = "TREAT"
	case "secondary":
		purpose = "COC"
	default:
		purpose = "TREAT"
	}

	return xacml.AuthzRequest{
		PatientBSN:             input.PatientBSN,
		HealthcareFacilityType: input.DataHolderFacilityType,
		AuthorInstitutionID:    input.DataHolderOrganizationUra,
		// This code is always the same, it's the code for _de gesloten vraag_
		EventCode:              "GGC002",
		SubjectRole:            input.RequestingUziRoleCode,
		ProviderID:             input.RequestingPractitionerIdentifier,
		ProviderInstitutionID:  input.RequestingOrganizationUra,
		ConsultingFacilityType: input.RequestingFacilityType,
		PurposeOfUse:           purpose,
	}
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
		input.ResourceId,
		input.ResourceType,
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
