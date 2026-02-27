package pdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/open-policy-agent/opa/v1/sdk"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type PDPInput struct {
	Subject PolicySubject `json:"subject"`
	Request HTTPRequest   `json:"request"`
	Context PDPContext    `json:"context"`
}

type PDPSubject struct {
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

var _ json.Unmarshaler = (*PDPSubject)(nil)
var _ json.Marshaler = (*PDPSubject)(nil)

func (s *PDPSubject) UnmarshalJSON(data []byte) error {
	type Alias PDPSubject
	var tmp Alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	tmp.OtherProps = make(map[string]any)
	if err := json.Unmarshal(data, &tmp.OtherProps); err != nil {
		return err
	}
	// remove standard properties from OtherProps
	delete(tmp.OtherProps, "client_id")
	delete(tmp.OtherProps, "client_qualifications")
	delete(tmp.OtherProps, "subject_id")
	delete(tmp.OtherProps, "subject_organization_id")
	delete(tmp.OtherProps, "subject_organization")
	delete(tmp.OtherProps, "subject_facility_type")
	delete(tmp.OtherProps, "subject_role")
	*s = PDPSubject(tmp)
	return nil
}

func (s PDPSubject) MarshalJSON() ([]byte, error) {
	type Alias PDPSubject
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

type OtherPDPSubject map[string]any

type HTTPRequest struct {
	Method      string      `json:"method"`
	Protocol    string      `json:"protocol"` // "HTTP/1.0"
	Path        string      `json:"path"`
	QueryParams url.Values  `json:"query_params"`
	Header      http.Header `json:"header"`
	Body        string      `json:"body"`
}

type PDPContext struct {
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

type PolicySubject struct {
	Client       PolicySubjectClient       `json:"client"`
	Organization PolicySubjectOrganization `json:"organization"`
	User         PolicySubjectUser         `json:"user"`
	OtherProps   map[string]any            `json:"-"`
}

type PolicySubjectClient struct {
	Id    string `json:"id"`
	Scope string `json:"scope"`
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

func NewPolicySubject(pdpSubject PDPSubject) PolicySubject {

	var policySubject PolicySubject
	policySubject.Client.Id = pdpSubject.ClientId
	policySubject.Client.Scope = pdpSubject.Scope

	policySubject.User.Id = pdpSubject.UserId
	policySubject.User.Role = pdpSubject.UserRole

	policySubject.Organization.Ura = pdpSubject.OrganizationUra
	policySubject.Organization.Name = pdpSubject.OrganizationName
	policySubject.Organization.FacilityType = pdpSubject.OrganizationFacilityType

	policySubject.OtherProps = pdpSubject.OtherProps

	return policySubject
}

type PolicyResource struct {
	Type       *fhir.ResourceType       `json:"type"`
	Properties PolicyResourceProperties `json:"properties"`
}

type PolicyResourceProperties struct {
	ResourceId string `json:"resource_id"`
	VersionId  string `json:"version_id"`
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

type PDPRequest struct {
	Input PDPInput `json:"input"`
}

type PDPResponse struct {
	Result PolicyResult `json:"result"`
}

type PolicyResult struct {
	Policy  string         `json:"policy"`
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

func (p *PolicyResult) AddReasons(input []string, format string, code TypeResultCode) {
	isNewSlice := cap(p.Reasons) == 0
	if isNewSlice {
		p.Reasons = make([]ResultReason, len(input))
	}

	for i, str := range input {
		reason := ResultReason{
			Code:        code,
			Description: fmt.Sprintf(format, str),
		}

		if isNewSlice {
			p.Reasons[i] = reason
		} else {
			p.Reasons = append(p.Reasons, reason)
		}
	}
}

// Allow helper for creating an allowed result without reasons
func Allow() PolicyResult {
	return PolicyResult{
		Allow: true,
	}
}

// Deny Helper for creating a result with a single deny reason
func Deny(reason ResultReason) PolicyResult {
	return PolicyResult{
		Allow: false,
		Reasons: []ResultReason{
			reason,
		},
	}
}

type TypeResultCode string

const (
	TypeResultCodeMissingRequiredValue TypeResultCode = "missing_required_value"
	TypeResultCodeUnexpectedInput      TypeResultCode = "unexpected_input"
	TypeResultCodeNotAllowed           TypeResultCode = "not_allowed"
	TypeResultCodeNotImplemented       TypeResultCode = "not_implemented"
	TypeResultCodeInternalError        TypeResultCode = "internal_error"
	TypeResultCodePIPError             TypeResultCode = "pip_error"
	TypeResultCodeInformational        TypeResultCode = "info"
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
