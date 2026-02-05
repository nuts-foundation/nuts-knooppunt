package pdp

import (
	"fmt"
	"net/http"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/open-policy-agent/opa/v1/sdk"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type PDPInput struct {
	Subject Subject     `json:"subject"`
	Request HTTPRequest `json:"request"`
	Context PDPContext  `json:"context"`
}

type Subject struct {
	Type       string            `json:"type"`
	Id         string            `json:"id"`
	Properties SubjectProperties `json:"properties"`
}

type SubjectProperties struct {
	ClientId              string   `json:"client_id"`
	ClientQualifications  []string `json:"client_qualifications"`
	SubjectId             string   `json:"subject_id"`
	SubjectOrganizationId string   `json:"subject_organization_id"`
	SubjectOrganization   string   `json:"subject_organization"`
	SubjectFacilityType   string   `json:"subject_facility_type"`
	SubjectRole           string   `json:"subject_role"`
}

type HTTPRequest struct {
	Method      string              `json:"method"`
	Protocol    string              `json:"protocol"` // "HTTP/1.0"
	Path        string              `json:"path"`
	QueryParams map[string][]string `json:"query_params"`
	Header      http.Header         `json:"header"`
	Body        string              `json:"body"`
}

type PDPContext struct {
	ConnectionTypeCode       string `json:"connection_type_code"`
	DataHolderFacilityType   string `json:"data_holder_facility_type"`
	DataHolderOrganizationId string `json:"data_holder_organization_id"`
	PatientBSN               string `json:"patient_bsn"`
}

type PolicyInput struct {
	Subject  Subject        `json:"subject"`
	Resource PolicyResource `json:"resource"`
	Action   PolicyAction   `json:"action"`
	Context  PolicyContext  `json:"context"`
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
	Name       string                 `json:"name"`
	Properties PolicyActionProperties `json:"properties"`
}

type PolicyActionProperties struct {
	Request        HTTPRequest          `json:"request"`
	ConnectionData PolicyConnectionData `json:"connection_data"`
}

type PolicyConnectionData struct {
	FHIRRest FhirConnectionData `json:"fhir_rest"`
}

type FhirConnectionData struct {
	isFHIRRest        bool                        `json:"is_fhir_rest"`
	CapabilityChecked bool                        `json:"capability_checked"`
	Include           []string                    `json:"include"`
	InteractionType   fhir.TypeRestfulInteraction `json:"interaction_type"`
	Operation         *string                     `json:"operation"`
	Revinclude        []string                    `json:"revinclude"`
	SearchParams      map[string]string           `json:"search_params"`
	PatientID         string                      `json:"patient_id"`
}

type PolicyContext struct {
	DataHolderFacilityType   string `json:"data_holder_facility_type"`
	DataHolderOrganizationId string `json:"data_holder_organization_id"`
	MitzConsent              bool   `json:"mitz_consent"`
	PatientBSN               string `json:"patient_bsn"`
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

func appendReasons(mainResult PolicyResult, results ...PolicyResult) PolicyResult {
	reasons := mainResult.Reasons
	for _, result := range results {
		reasons = append(reasons, result.Reasons...)
	}
	mainResult.Reasons = reasons
	return mainResult
}

type TypeResultCode string

const (
	TypeResultCodeMissingRequiredValue TypeResultCode = "missing_required_value"
	TypeResultCodeUnexpectedInput      TypeResultCode = "unexpected_input"
	TypeResultCodeNotAllowed           TypeResultCode = "not_allowed"
	TypeResultCodeNotImplemented       TypeResultCode = "not_implemented"
	TypeResultCodeInternalError        TypeResultCode = "internal_error"
	TypeResultCodePIPError             TypeResultCode = "pip_error"
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
