package pdp

import (
	"fmt"
	"net/http"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
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
	DataHolderOrganizationId string `json:"data_holder_organization_id"`
	DataHolderFacilityType   string `json:"data_holder_facility_type"`
}

type PolicyInput struct {
	Subject  Subject        `json:"subject"`
	Resource PolicyResource `json:"resource"`
	Action   PolicyAction   `json:"action"`
	Context  PolicyContext  `json:"context"`
}

type PolicyResource struct {
	Type       fhir.ResourceType        `json:"type"`
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
	InteractionType fhir.TypeRestfulInteraction `json:"interaction_type"`
	Operation       *string                     `json:"operation"`
	SearchParams    []string                    `json:"search_params"`
	Include         []string                    `json:"include"`
	Revinclude      []string                    `json:"revinclude"`
}

type PolicyPatient struct {
	PatientID   string `json:"patient_id"`
	PatientBSN  string `json:"patient_bsn"`
	MitzConsent bool   `json:"mitz_consent"`
}

type PolicyContext struct {
	DataHolderFacilityType   string          `json:"data_holder_facility_type"`
	DataHolderOrganizationId string          `json:"data_holder_organization_id"`
	Patients                 []PolicyPatient `json:"patients"`
	PurposeOfUse             string          `json:"purpose_of_use"`
}

type PDPRequest struct {
	Input PDPInput `json:"input"`
}

type PDPResponse struct {
	Result PolicyResult `json:"result"`
}

type PolicyResult struct {
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

type TypeResultCode string

const (
	TypeResultCodeMissingRequiredValue TypeResultCode = "missing_required_value"
	TypeResultCodeUnexpectedInput      TypeResultCode = "unexpected_input"
	TypeResultCodeNotAllowed           TypeResultCode = "not_allowed"
	TypeResultCodeNotImplemented       TypeResultCode = "not_implemented"
	TypeResultCodeInternalError        TypeResultCode = "internal_error"
)

type PIPConfig struct {
	URL string
}

type Config struct {
	Enabled bool      `koanf:"enabled"`
	PIP     PIPConfig `koanf:"pipurl"`
}

type Component struct {
	Config    Config
	Mitz      *mitz.Component
	pipClient fhirclient.Client
}
