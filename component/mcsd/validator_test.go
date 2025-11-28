package mcsd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Test that the generic validators do their job. At this point, this only checks whether
// the resource is of an allowed type before deferring to a resource-specific function.
func TestGenericValidators(t *testing.T) {
	resources := []struct {
		name         string
		resourceJSON []byte
		valid        bool
	}{
		{
			name:         "valid Organization",
			resourceJSON: []byte(`{"resourceType":"Organization", "identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/ura","value":"12345"}]}`),
			valid:        true,
		},
		{
			name:         "invalid resource type",
			resourceJSON: []byte(`{"resourceType":"Patient"}`),
			valid:        false,
		},
	}

	for _, tt := range resources {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdate(t.Context(), ValidationRules{AllowedResourceTypes: []string{"Organization"}}, tt.resourceJSON, make(map[*fhir.Organization][]*fhir.Organization))

			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}

}

// Test validator for Organization resources. An organization should either have an URA number
// or should be a sub-organization to an Organization with an URA number.
//
// https://nuts-foundation.github.io/nl-generic-functions-ig/care-services.html#update-client
func TestOrganizationValidator(t *testing.T) {
	organizations := []struct {
		name         string
		organization fhir.Organization
		valid        bool
	}{
		{
			name: "valid Organization",
			organization: fhir.Organization{
				Identifier: []fhir.Identifier{
					{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("12345"),
					},
				},
			},
			valid: true,
		},
		{
			name: "Organization with double URA",
			organization: fhir.Organization{
				Identifier: []fhir.Identifier{
					{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("12346"),
					},
					{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("12345"),
					},
				},
			},
			valid: false,
		},
		{
			name:         "Organization without Identifier",
			organization: fhir.Organization{},
			valid:        false,
		},
		{
			name: "Organization with non-URA Identifier",
			organization: fhir.Organization{
				Identifier: []fhir.Identifier{
					{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/uzi-nr-sys"),
						Value:  to.Ptr("12345"),
					},
				},
			},
			valid: false,
		},
		{
			name: "Organization with URA and non-URA Identifier",
			organization: fhir.Organization{
				Identifier: []fhir.Identifier{
					{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/uzi-nr-sys"),
						Value:  to.Ptr("12345"),
					},
					{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("12345"),
					},
				},
			},
			valid: true,
		},
	}

	for _, tt := range organizations {
		t.Run(tt.name, func(t *testing.T) {
			// Test with no parent organization (expects organization to have its own URA or no partOf)
			err := validateOrganizationResource(&tt.organization)

			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestAssertReferencePointsToValidOrganization(t *testing.T) {
	tests := []struct {
		name               string
		reference          *fhir.Reference
		parentOrganization *fhir.Organization
		allOrganizations   []*fhir.Organization
		shouldSucceed      bool
		description        string
	}{
		{
			name: "reference to parent organization",
			reference: &fhir.Reference{
				Reference: to.Ptr("Organization/parent-org"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    true,
			description:      "should succeed when reference matches parent organization",
		},
		{
			name: "reference to organization in allOrganizations",
			reference: &fhir.Reference{
				Reference: to.Ptr("Organization/child-org-1"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{
				{Id: to.Ptr("child-org-1")},
				{Id: to.Ptr("child-org-2")},
			},
			shouldSucceed: true,
			description:   "should succeed when reference matches one of the allOrganizations",
		},
		{
			name: "reference to second organization in allOrganizations",
			reference: &fhir.Reference{
				Reference: to.Ptr("Organization/child-org-2"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{
				{Id: to.Ptr("child-org-1")},
				{Id: to.Ptr("child-org-2")},
			},
			shouldSucceed: true,
			description:   "should succeed when reference matches second organization in allOrganizations",
		},
		{
			name: "reference to invalid organization",
			reference: &fhir.Reference{
				Reference: to.Ptr("Organization/invalid-org"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{
				{Id: to.Ptr("child-org-1")},
			},
			shouldSucceed: false,
			description:   "should fail when reference does not match any organization",
		},
		{
			name:      "nil reference",
			reference: nil,
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    false,
			description:      "should fail when reference is nil",
		},
		{
			name: "reference with no valid organizations",
			reference: &fhir.Reference{
				Reference: to.Ptr("Organization/some-org"),
			},
			parentOrganization: nil,
			allOrganizations:   []*fhir.Organization{},
			shouldSucceed:      false,
			description:        "should fail when no valid organizations are provided",
		},
		{
			name: "reference without slash",
			reference: &fhir.Reference{
				Reference: to.Ptr("parent-org"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    true,
			description:      "should succeed with reference without slash (direct ID)",
		},
		{
			name: "empty allOrganizations with parent match",
			reference: &fhir.Reference{
				Reference: to.Ptr("Organization/parent-org"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    true,
			description:      "should succeed when reference matches parent even with empty allOrganizations",
		},
		{
			name: "reference with absolute URL",
			reference: &fhir.Reference{
				Reference: to.Ptr("http://example.org/fhir/Organization/child-org-1"),
			},
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{
				{Id: to.Ptr("child-org-1")},
			},
			shouldSucceed: true,
			description:   "should succeed when reference is an absolute URL and ID matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a parentOrganizationMap with the test data
			parentOrgMap := make(map[*fhir.Organization][]*fhir.Organization)
			if tt.parentOrganization != nil {
				parentOrgMap[tt.parentOrganization] = tt.allOrganizations
			}

			err := assertReferencePointsToValidOrganization(tt.reference, parentOrgMap, "test.reference")

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestAssertOrganizationHasEndpointReference(t *testing.T) {
	endpointID := to.Ptr("endpoint-1")
	endpointID2 := to.Ptr("endpoint-2")

	tests := []struct {
		name               string
		endpointID         *string
		parentOrganization *fhir.Organization
		allOrganizations   []*fhir.Organization
		shouldSucceed      bool
		description        string
	}{
		{
			name:       "parent organization has endpoint reference",
			endpointID: endpointID,
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
				Endpoint: []fhir.Reference{
					{Reference: to.Ptr("Endpoint/endpoint-1")},
				},
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    true,
			description:      "should succeed when parent org has the endpoint",
		},
		{
			name:       "organization in allOrganizations has endpoint reference",
			endpointID: endpointID,
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
			},
			allOrganizations: []*fhir.Organization{
				{
					Id: to.Ptr("child-org-1"),
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-1")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when one of allOrganizations has the endpoint",
		},
		{
			name:       "endpoint not referenced by any organization",
			endpointID: endpointID,
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
				Endpoint: []fhir.Reference{
					{Reference: to.Ptr("Endpoint/endpoint-2")},
				},
			},
			allOrganizations: []*fhir.Organization{
				{
					Id: to.Ptr("child-org-1"),
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-2")},
					},
				},
			},
			shouldSucceed: false,
			description:   "should fail when no organization references this endpoint",
		},
		{
			name:       "multiple endpoints, target endpoint present",
			endpointID: endpointID2,
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
				Endpoint: []fhir.Reference{
					{Reference: to.Ptr("Endpoint/endpoint-1")},
					{Reference: to.Ptr("Endpoint/endpoint-2")},
				},
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    true,
			description:      "should succeed when parent org has target endpoint among multiple",
		},
		{
			name:               "no organizations provided",
			endpointID:         endpointID,
			parentOrganization: nil,
			allOrganizations:   []*fhir.Organization{},
			shouldSucceed:      false,
			description:        "should fail when no organizations are provided",
		},
		{
			name:       "endpoint with absolute URL reference",
			endpointID: endpointID,
			parentOrganization: &fhir.Organization{
				Id: to.Ptr("parent-org"),
				Endpoint: []fhir.Reference{
					{Reference: to.Ptr("http://example.org/fhir/Endpoint/endpoint-1")},
				},
			},
			allOrganizations: []*fhir.Organization{},
			shouldSucceed:    true,
			description:      "should succeed when organization references endpoint with absolute URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a parentOrganizationMap with the test data
			parentOrgMap := make(map[*fhir.Organization][]*fhir.Organization)
			if tt.parentOrganization != nil {
				parentOrgMap[tt.parentOrganization] = tt.allOrganizations
			}

			err := assertOrganizationHasEndpointReference(tt.endpointID, parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}
