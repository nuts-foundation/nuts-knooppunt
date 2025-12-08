package pdp

import (
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type AuthData struct {
	Scope                            string `json:"scope"`
	RequestingUra                    string `json:"requesting_ura"`
	RequestingFacilityType           string `json:"requesting_facility_type"`
	RequestingPractitionerIdentifier string `json:"requesting_practitioner_identifier"`
	RequestingUziRoleCode            string `json:"requesting_uzi_role_code"`
}

type HTTPRequest struct {
	Method      string              `json:"method"`
	Protocol    string              `json:"protocol"` // "HTTP/1.0"
	Path        string              `json:"path"`
	QueryParams map[string][]string `json:"query_params"`
	Header      http.Header         `json:"header"`
	Body        string              `json:"body"`
}

type PolicyContext struct {
	DataHolderUra          string `json:"data_holder_ura"`
	DataHolderFacilityType string `json:"data_holder_facility_type"`
}

type PDPInput struct {
	AuthData
	HTTPRequest
	PolicyContext
}

type FHIRRequest struct {
	InteractionType fhir.TypeRestfulInteraction `json:"interaction_type"`
	ResourceType    fhir.ResourceType           `json:"resource_type"`
	SearchParams    []string                    `json:"search_params"`
	ResourceId      string                      `json:"resource_id"`
	Include         []string                    `json:"include"`
	Revinclude      []string                    `json:"revinclude"`
}

type MainPolicyInput struct {
	AuthData
	FHIRRequest
	PolicyContext
}

type MainPolicyRequest struct {
	Input MainPolicyInput `json:"input"`
}

type MainPolicyResponse struct {
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

type Config struct {
	Enabled bool
}

type Component struct {
	Config Config
	Mitz   *mitz.Component
}
