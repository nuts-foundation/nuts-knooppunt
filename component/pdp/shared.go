package pdp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type MainPolicyInput struct {
	Scope                            string                      `json:"scope"`
	Method                           string                      `json:"method"`
	Path                             []string                    `json:"path"`
	PatientBSN                       string                      `json:"patient_bsn"`
	PurposeOfUse                     string                      `json:"purpose_of_use"`
	DataHolderFacilityType           string                      `json:"data_holder_facility_type"`
	DataHolderOrganizationUra        string                      `json:"data_holder_organization_ura"`
	RequestingFacilityType           string                      `json:"requesting_facility_type"`
	RequestingOrganizationUra        string                      `json:"requesting_organization_ura"`
	RequestingPractitionerIdentifier string                      `json:"requesting_practitioner_identifier"`
	RequestingUziRoleCode            string                      `json:"requesting_uzi_role_code"`
	InteractionType                  fhir.TypeRestfulInteraction `json:"interaction_type"`
	ResourceType                     fhir.ResourceType           `json:"resource_type"`
	SearchParams                     []string                    `json:"search_params"`
	ResourceId                       string                      `json:"resource_id"`
	Include                          []string                    `json:"include"`
	Revinclude                       []string                    `json:"revinclude"`
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

// ManyReasons Helper for easily adding multiple of reasons of the same type
func ManyReasons(target *[]ResultReason, input []string, format string, code TypeResultCode) {
	for _, str := range input {
		reason := ResultReason{
			Code:        code,
			Description: fmt.Sprintf(format, str),
		}
		*target = append(*target, reason)
	}
}

type TypeResultCode int

const (
	TypeResultCodeMissingRequiredValue TypeResultCode = iota
	TypeResultCodeUnexpectedInput
	TypeResultCodeNotAllowed
	TypeResultCodeNotImplemented
	TypeResultCodeInternalError
)

func (code TypeResultCode) MarshalJSON() ([]byte, error) {
	buffer := bytes.Buffer{}
	enc := json.NewEncoder(&buffer)
	enc.SetEscapeHTML(false)
	err := enc.Encode(code.Code())
	return buffer.Bytes(), err
}
func (code TypeResultCode) Code() string {
	switch code {
	case TypeResultCodeMissingRequiredValue:
		return "missing_required_value"
	case TypeResultCodeUnexpectedInput:
		return "unexpected_input"
	case TypeResultCodeNotAllowed:
		return "not_allowed"
	case TypeResultCodeNotImplemented:
		return "not_implemented"
	case TypeResultCodeInternalError:
		return "internal_error"
	}
	return "<unknown>"
}
func (code *TypeResultCode) UnmarshalJSON(json []byte) error {
	s := strings.Trim(string(json), "\"")
	switch s {
	case "missing_required_value":
		*code = TypeResultCodeMissingRequiredValue
	case "unexpected_input":
		*code = TypeResultCodeUnexpectedInput
	case "not_allowed":
		*code = TypeResultCodeNotAllowed
	case "not_implemented":
		*code = TypeResultCodeNotImplemented
	case "internal_error":
		*code = TypeResultCodeInternalError
	default:
		return fmt.Errorf("unknown TypeRestfulInteraction code `%s`", s)
	}
	return nil
}

type Config struct {
	Enabled bool
}

type Component struct {
	Config Config
	Mitz   *mitz.Component
}
