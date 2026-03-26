package pdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/mitchellh/copystructure"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/open-policy-agent/opa/v1/sdk"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type APIInput struct {
	Subject APISubject  `json:"subject"`
	Request HTTPRequest `json:"request"`
	Context APIContext  `json:"context"`
}

type APISubject struct {
	OtherProps               map[string]any `json:"-"`
	Active                   bool           `json:"active"`
	ClientId                 string         `json:"client_id"`
	Scope                    string         `json:"scope"`
	UserId                   string         `json:"user_id"`
	UserRole                 string         `json:"user_role"`
	OrganizationUra          string         `json:"organization_ura"`
	OrganizationName         string         `json:"organization_name"`
	OrganizationFacilityType string         `json:"organization_facility_type"`
}

var _ json.Unmarshaler = (*APISubject)(nil)
var _ json.Marshaler = (*APISubject)(nil)

func (s *APISubject) UnmarshalJSON(data []byte) error {
	type Alias APISubject
	var tmp Alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	tmp.OtherProps = make(map[string]any)
	if err := json.Unmarshal(data, &tmp.OtherProps); err != nil {
		return err
	}
	// remove standard properties from OtherProps
	delete(tmp.OtherProps, "active")
	delete(tmp.OtherProps, "client_id")
	delete(tmp.OtherProps, "scope")
	delete(tmp.OtherProps, "user_id")
	delete(tmp.OtherProps, "user_role")
	delete(tmp.OtherProps, "organization_ura")
	delete(tmp.OtherProps, "organization_name")
	delete(tmp.OtherProps, "organization_facility_type")
	*s = APISubject(tmp)
	return nil
}

func (s APISubject) MarshalJSON() ([]byte, error) {
	type Alias APISubject
	tmp := Alias(s)
	data, err := json.Marshal(tmp)
	if err != nil {
		return nil, err
	}
	if len(s.OtherProps) == 0 {
		return data, nil
	}
	var baseMap map[string]any
	if err := json.Unmarshal(data, &baseMap); err != nil {
		return nil, err
	}
	for k, v := range s.OtherProps {
		baseMap[k] = v
	}
	return json.Marshal(baseMap)
}

type HTTPRequest struct {
	Method      string      `json:"method"`
	Protocol    string      `json:"protocol"` // "HTTP/1.0"
	Path        string      `json:"path"`
	QueryParams url.Values  `json:"query_params"`
	Header      http.Header `json:"header"`
	Body        string      `json:"body"`
}

type APIContext struct {
	ConnectionTypeCode       string `json:"connection_type_code"`
	DataHolderFacilityType   string `json:"data_holder_facility_type"`
	DataHolderOrganizationId string `json:"data_holder_organization_id"`
	PatientBSN               string `json:"patient_bsn"`
}

type PolicyInput struct {
	Subject  PolicySubject  `json:"subject"`
	Resource PolicyResource `json:"resource"`
	Action   PolicyAction   `json:"action"`
	Context  PolicyContext  `json:"context"`
}

type OtherProps map[string]any

type PolicySubject struct {
	OtherProps   `json:"-"`
	Client       PolicySubjectClient       `json:"client"`
	Organization PolicySubjectOrganization `json:"organization"`
	User         PolicySubjectUser         `json:"user"`
}

func (s PolicySubject) MarshalJSON() ([]byte, error) {
	type Alias PolicySubject
	tmp := Alias(s)
	data, err := json.Marshal(tmp)
	if err != nil {
		return nil, err
	}
	if len(s.OtherProps) == 0 {
		return data, nil
	}
	var baseMap map[string]any
	if err := json.Unmarshal(data, &baseMap); err != nil {
		return nil, err
	}
	for k, v := range s.OtherProps {
		baseMap[k] = v
	}
	return json.Marshal(baseMap)
}

type PolicySubjectClient struct {
	Id     string   `json:"id"`
	Scopes []string `json:"scopes"`
}
type PolicySubjectOrganization struct {
	Ura          string `json:"ura"`
	Name         string `json:"name"`
	FacilityType string `json:"facility_type"`
}
type PolicySubjectUser struct {
	Id   string `json:"id"`
	Role string `json:"role"`
}

func NewPolicySubject(apiSubject APISubject) PolicySubject {

	var policySubject PolicySubject
	policySubject.Client.Id = apiSubject.ClientId
	policySubject.Client.Scopes = strings.Fields(apiSubject.Scope)

	policySubject.User.Id = apiSubject.UserId
	policySubject.User.Role = apiSubject.UserRole

	policySubject.Organization.Ura = apiSubject.OrganizationUra
	policySubject.Organization.Name = apiSubject.OrganizationName
	policySubject.Organization.FacilityType = apiSubject.OrganizationFacilityType

	policySubject.OtherProps = apiSubject.OtherProps

	return policySubject
}

func (p PolicyInput) Copy() PolicyInput {
	result, err := copystructure.Copy(p)
	if err != nil {
		panic(fmt.Sprintf("failed to copy PolicyInput: %v", err))
	}
	return result.(PolicyInput)
}

type PolicyResource struct {
	Id        string             `json:"id"`
	Type      *fhir.ResourceType `json:"type"`
	VersionId string             `json:"version_id"`
	Content   map[string]any     `json:"content,omitempty"`
	Consents  []PolicyConsent    `json:"consents"`
}

type PolicyConsent struct {
	Scope string `json:"scope"`
}

type PolicyAction struct {
	Name               string       `json:"name"`
	ConnectionTypeCode string       `json:"connection_type_code"`
	Request            HTTPRequest  `json:"request"`
	FHIRRest           FHIRRestData `json:"fhir_rest"`
}

type FHIRRestData struct {
	CapabilityChecked bool                        `json:"capability_checked"`
	Include           []string                    `json:"include"`
	InteractionType   fhir.TypeRestfulInteraction `json:"interaction_type"`
	Operation         *string                     `json:"operation"`
	Revinclude        []string                    `json:"revinclude"`
	SearchParams      map[string][]string         `json:"search_params"`
}

type PolicyContext struct {
	DataHolderFacilityType   string `json:"data_holder_facility_type"`
	DataHolderOrganizationId string `json:"data_holder_organization_id"`
	MitzConsent              bool   `json:"mitz_consent"`
	PatientBSN               string `json:"patient_bsn"`
	PatientID                string `json:"patient_id"`
	PurposeOfUse             string `json:"purpose_of_use"`
}

type APIRequest struct {
	Input APIInput `json:"input"`
}

type APIResponse struct {
	Allow bool `json:"allow"`
	// Error is an optional field that can be used to provide additional information about why a decision couldn't be made.
	// This is intended for informational purposes and should not be used to determine the outcome of the decision (i.e. allow/deny).
	Error    string                  `json:"error,omitempty"`
	Policies map[string]PolicyResult `json:"policies"`
}

type PolicyResult struct {
	Allow   bool           `json:"allow"`
	Reasons []ResultReason `json:"reasons"`
}

type ResultReason struct {
	Code        TypeResultCode `json:"code"`
	Description string         `json:"description"`
}

func (r ResultReason) String() string {
	return fmt.Sprintf("%s - %s", r.Code, r.Description)
}

type TypeResultCode string

const (
	TypeResultCodeUnexpectedInput TypeResultCode = "unexpected_input"
	TypeResultCodeNotAllowed      TypeResultCode = "not_allowed"
	TypeResultCodeNotImplemented  TypeResultCode = "not_implemented"
	TypeResultCodeInternalError   TypeResultCode = "internal_error"
	TypeResultCodePIPError        TypeResultCode = "pip_error"
	TypeResultCodeInformational   TypeResultCode = "info"
)

type PIPConfig struct {
	URL string `koanf:"url"`
}

type Config struct {
	Enabled bool      `koanf:"enabled"`
	PIP     PIPConfig `koanf:"pip"`
}

type Component struct {
	Config           Config
	consentChecker   mitz.ConsentChecker
	pipClient        fhirclient.Client
	opaService       *sdk.OPA
	opaBundleBaseURL string
}
