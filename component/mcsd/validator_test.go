package mcsd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

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
			err2 := validateOrganizationResource(t.Context(), &tt.organization)

			if tt.valid {
				require.NoError(t, err2)
			} else {
				require.Error(t, err2)
			}
		})
	}
}
