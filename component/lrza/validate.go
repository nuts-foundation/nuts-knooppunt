package lrza

import (
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Local validation against the GF Care Service Directory profiles: the cardinalities and invariants
// a client most likely gets wrong, checked by hand and structurally. It exists to give the client a
// complete list of problems up front, in one response. The LRZA stays the final validator - value
// sets are only checked on code system here, full memberOf is left to it - and its own 422 is passed
// through. (mcsd's validator is not reused: it checks the legacy URA system, and rules specific to
// the mCSD update client.)

// validationIssue is one problem found, at a FHIRPath expression.
type validationIssue struct {
	code       fhir.IssueType
	expression string
	message    string
}

// validationError is the 422 for failed local validation, with one issue per problem.
func validationError(issues []validationIssue) error {
	outcome := fhir.OperationOutcome{}
	for _, issue := range issues {
		outcome.Issue = append(outcome.Issue, errorIssue(issue.code, issue.message, issue.expression))
	}
	return &registrationError{status: http.StatusUnprocessableEntity, outcome: outcome}
}

// connectionTypeSystems are the code systems of nl-gf-connection-types-vs.
var connectionTypeSystems = []string{coding.EndpointConnectionTypeSystem, coding.CSDAuthorizationServerSystem}

// payloadTypeSystems are the code systems of nl-gf-payload-type-vs.
var payloadTypeSystems = []string{coding.CSDDataCategoriesSystem, coding.EndpointPayloadTypeSystem}

// assignerSystems are the identifier systems the IG accepts for the assigner of a custodian-assigned
// identifier.
var assignerSystems = []string{coding.URAOIDNamingSystem, coding.KVKOIDNamingSystem}

// validateResource validates a resource about to be written, returning the problems found.
func validateResource(res resource) []validationIssue {
	v := &validator{resourceType: res.resourceType()}
	v.prohibited(res, "implicitRules")
	v.noModifierExtensions(res, res.resourceType())
	v.assignerSystems(res)
	switch res.resourceType() {
	case "Organization":
		v.required(res, "identifier", "name", "type")
		uras := uraValues(res)
		if len(uras) > 1 {
			v.add(fhir.IssueTypeInvariant, "Organization.identifier", "an Organization has at most one URA identifier")
		}
		if len(uras) == 0 && !present(res["partOf"]) {
			v.add(fhir.IssueTypeInvariant, "Organization", "ura-identifier-or-partof: an Organization has a URA identifier or is partOf another Organization")
		}
		if len(uras) == 0 && present(res["endpoint"]) {
			v.add(fhir.IssueTypeInvariant, "Organization.endpoint", "only-ura-identified-organization-can-refer-to-an-endpoint: only an Organization with a URA identifier can refer to an Endpoint")
		}
		v.noHomeUse(res, "address", "org-2: an address of an Organization can never be of use 'home'")
		v.noHomeUse(res, "telecom", "org-3: the telecom of an Organization can never be of use 'home'")
	case "Location":
		v.required(res, "status", "name", "type", "managingOrganization")
	case "HealthcareService":
		v.required(res, "identifier", "providedBy", "type", "name")
	case "Endpoint":
		v.required(res, "status", "connectionType", "payloadType", "address", "managingOrganization")
		if connectionType, ok := res["connectionType"].(map[string]any); ok && !inSystems(connectionType["system"], connectionTypeSystems) {
			v.add(fhir.IssueTypeValue, "Endpoint.connectionType", fmt.Sprintf("connectionType must be a code from %v", connectionTypeSystems))
		}
		payloadTypes, _ := res["payloadType"].([]any)
		for i, item := range payloadTypes {
			concept, _ := item.(map[string]any)
			if !hasCodingFrom(concept, payloadTypeSystems) {
				v.add(fhir.IssueTypeValue, fmt.Sprintf("Endpoint.payloadType[%d]", i), fmt.Sprintf("payloadType must have a coding from %v", payloadTypeSystems))
			}
		}
	}
	return v.issues
}

type validator struct {
	resourceType string
	issues       []validationIssue
}

func (v *validator) add(code fhir.IssueType, expression, message string) {
	v.issues = append(v.issues, validationIssue{code: code, expression: expression, message: message})
}

// required reports each of the elements that is missing or empty.
func (v *validator) required(res resource, elements ...string) {
	for _, element := range elements {
		if !present(res[element]) {
			v.add(fhir.IssueTypeRequired, v.resourceType+"."+element, fmt.Sprintf("%s.%s is required", v.resourceType, element))
		}
	}
}

func (v *validator) prohibited(res resource, element string) {
	if _, ok := res[element]; ok {
		v.add(fhir.IssueTypeStructure, v.resourceType+"."+element, fmt.Sprintf("%s.%s is not allowed", v.resourceType, element))
	}
}

// noModifierExtensions reports a modifierExtension anywhere in the resource: the profiles prohibit
// them, as a modifier changes the meaning of the resource in ways a consumer can't know about.
func (v *validator) noModifierExtensions(value any, path string) {
	switch typed := value.(type) {
	case resource:
		v.noModifierExtensions(map[string]any(typed), path)
	case map[string]any:
		for key, child := range typed {
			if key == "modifierExtension" {
				v.add(fhir.IssueTypeStructure, path+".modifierExtension", "modifierExtension is not allowed")
				continue
			}
			v.noModifierExtensions(child, path+"."+key)
		}
	case []any:
		for i, child := range typed {
			v.noModifierExtensions(child, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

// assignerSystems checks the assigner-identifier-system invariant: the assigner of an identifier is
// identified by URA or KvK.
func (v *validator) assignerSystems(res resource) {
	for i, identifier := range identifiers(res) {
		assigner, _ := identifier["assigner"].(map[string]any)
		assignerIdentifier, _ := assigner["identifier"].(map[string]any)
		if assignerIdentifier == nil {
			continue
		}
		if !inSystems(assignerIdentifier["system"], assignerSystems) {
			v.add(fhir.IssueTypeInvariant, fmt.Sprintf("%s.identifier[%d].assigner.identifier.system", v.resourceType, i),
				fmt.Sprintf("assigner-identifier-system: the assigner of an identifier is identified by one of %v", assignerSystems))
		}
	}
}

// noHomeUse reports items of a list element with use 'home'.
func (v *validator) noHomeUse(res resource, element, message string) {
	items, _ := res[element].([]any)
	for i, item := range items {
		if typed, ok := item.(map[string]any); ok && typed["use"] == "home" {
			v.add(fhir.IssueTypeInvariant, fmt.Sprintf("%s.%s[%d].use", v.resourceType, element, i), message)
		}
	}
}

// present reports whether a JSON value is set: not null, and not an empty string, list or object.
func present(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func inSystems(system any, systems []string) bool {
	s, ok := system.(string)
	if !ok {
		return false
	}
	for _, candidate := range systems {
		if s == candidate {
			return true
		}
	}
	return false
}

// hasCodingFrom reports whether a CodeableConcept has a coding from one of the systems.
func hasCodingFrom(concept map[string]any, systems []string) bool {
	codings, _ := concept["coding"].([]any)
	for _, item := range codings {
		if c, ok := item.(map[string]any); ok && inSystems(c["system"], systems) {
			return true
		}
	}
	return false
}
