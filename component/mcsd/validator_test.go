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
			err := ValidateUpdate(t.Context(), ValidationRules{AllowedResourceTypes: []string{"Organization"}}, tt.resourceJSON)

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
			err := validateOrganizationResource(t.Context(), &tt.organization)

			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
