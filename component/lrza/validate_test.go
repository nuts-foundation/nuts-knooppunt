package lrza

import (
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/stretchr/testify/assert"
)

func TestValidateResource(t *testing.T) {
	validOrganization := func() resource {
		return resource{
			"resourceType": "Organization",
			"name":         "Sunflower",
			"type":         []any{map[string]any{"text": "care home"}},
			"identifier":   []any{uraIdentifierJSON(sunflowerURA)},
		}
	}
	department := func() resource {
		org := validOrganization()
		org["identifier"] = []any{map[string]any{"system": "http://example.org/departments", "value": "ward"}}
		org["partOf"] = organizationRef("sunflower")
		return org
	}
	with := func(res resource, element string, value any) resource {
		res[element] = value
		return res
	}
	without := func(res resource, element string) resource {
		delete(res, element)
		return res
	}

	for _, tc := range []struct {
		name       string
		res        resource
		expression string
		message    string
	}{
		{"valid Organization", validOrganization(), "", ""},
		{"valid department", department(), "", ""},
		{"valid Location", with(testLocation("", "sunflower"), "identifier", []any{}), "", ""},
		{"valid Endpoint", testEndpoint("", "sunflower"), "", ""},
		{"Organization.name is required", without(validOrganization(), "name"), "Organization.name", "required"},
		{"Organization.type is required", without(validOrganization(), "type"), "Organization.type", "required"},
		{"Location.managingOrganization is required", without(testLocation("", "sunflower"), "managingOrganization"), "Location.managingOrganization", "required"},
		{"Location.status is required", without(testLocation("", "sunflower"), "status"), "Location.status", "required"},
		{"HealthcareService.providedBy is required", resource{"resourceType": "HealthcareService", "identifier": []any{map[string]any{"value": "x"}}, "type": []any{map[string]any{"text": "x"}}, "name": "x"}, "HealthcareService.providedBy", "required"},
		{"Endpoint.address is required", without(testEndpoint("", "sunflower"), "address"), "Endpoint.address", "required"},
		{"implicitRules is not allowed", with(validOrganization(), "implicitRules", "http://example.org/rules"), "Organization.implicitRules", "not allowed"},
		{"modifierExtension is not allowed, also nested", with(validOrganization(), "contact", []any{map[string]any{"modifierExtension": []any{}}}), "Organization.contact[0].modifierExtension", "not allowed"},
		{"ura-identifier-or-partof", without(department(), "partOf"), "Organization", "ura-identifier-or-partof"},
		{"only-ura-identified-organization-can-refer-to-an-endpoint", with(department(), "endpoint", []any{map[string]any{"reference": "Endpoint/1"}}), "Organization.endpoint", "only-ura-identified-organization-can-refer-to-an-endpoint"},
		{"org-2", with(validOrganization(), "address", []any{map[string]any{"use": "home"}}), "Organization.address[0].use", "org-2"},
		{"org-3", with(validOrganization(), "telecom", []any{map[string]any{"use": "work"}, map[string]any{"use": "home"}}), "Organization.telecom[1].use", "org-3"},
		{"at most one URA", with(validOrganization(), "identifier", []any{uraIdentifierJSON(sunflowerURA), uraIdentifierJSON(care2CureURA)}), "Organization.identifier", "at most one URA"},
		{"assigner-identifier-system", with(validOrganization(), "identifier", []any{
			uraIdentifierJSON(sunflowerURA),
			map[string]any{"value": "x", "assigner": map[string]any{"identifier": map[string]any{"system": "http://example.org", "value": "y"}}},
		}), "Organization.identifier[1].assigner.identifier.system", "assigner-identifier-system"},
		{"connectionType code system", with(testEndpoint("", "sunflower"), "connectionType", map[string]any{"system": "http://example.org", "code": "x"}), "Endpoint.connectionType", "connectionType"},
		{"authorization server connectionType is accepted", with(testEndpoint("", "sunflower"), "connectionType", map[string]any{"system": coding.CSDAuthorizationServerSystem, "code": "oauth-nuts"}), "", ""},
		{"payloadType code system", with(testEndpoint("", "sunflower"), "payloadType", []any{
			map[string]any{"coding": []any{map[string]any{"system": coding.EndpointPayloadTypeSystem, "code": "none"}}},
			map[string]any{"coding": []any{map[string]any{"system": "http://example.org", "code": "x"}}},
		}), "Endpoint.payloadType[1]", "payloadType"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := validateResource(tc.res)
			if tc.expression == "" {
				assert.Empty(t, issues)
				return
			}
			if assert.Len(t, issues, 1) {
				assert.Equal(t, tc.expression, issues[0].expression)
				assert.Contains(t, issues[0].message, tc.message)
			}
		})
	}
}
