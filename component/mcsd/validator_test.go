package mcsd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

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
			// Create a parent organization map with the organization having a matching URA
			parentOrgMap := make(map[*fhir.Organization][]*fhir.Organization)
			if len(tt.organization.Identifier) > 0 {
				// Add a parent org with the same URA for validation
				parentOrg := &fhir.Organization{
					Id:         to.Ptr("parent-org"),
					Identifier: tt.organization.Identifier,
				}
				parentOrgMap[parentOrg] = []*fhir.Organization{}
			}

			err := validateOrganizationResource(&tt.organization, parentOrgMap)

			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateOrganizationResource_URAValidation(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"

	tests := []struct {
		name          string
		organization  *fhir.Organization
		parentOrgMap  map[*fhir.Organization][]*fhir.Organization
		shouldSucceed bool
		description   string
	}{
		{
			name: "organization with URA matching parent org",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when organization URA matches parent organization URA",
		},
		{
			name: "organization with URA not matching any parent org",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("99999")},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when organization URA does not match any parent organization URA",
		},
		{
			name: "organization with URA matching one of multiple parent orgs",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("67890")},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org-1"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
				{
					Id: to.Ptr("parent-org-2"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("67890")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when organization URA matches one of multiple parent organizations",
		},
		{
			name: "organization with multiple URA identifiers",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					{System: to.Ptr(uraSystem), Value: to.Ptr("67890")},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when organization has multiple URA identifiers",
		},
		{
			name: "organization with no URA and no partOf",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr("http://other-system.nl"), Value: to.Ptr("12345")},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when organization has no URA identifier and no partOf reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrganizationResource(tt.organization, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidateOrganizationResource_PartOfChainValidation(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"

	tests := []struct {
		name          string
		organization  *fhir.Organization
		parentOrgMap  map[*fhir.Organization][]*fhir.Organization
		shouldSucceed bool
		description   string
	}{
		{
			name: "organization with no URA but valid partOf to org with URA",
			organization: &fhir.Organization{
				Id:     to.Ptr("child-org"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/parent-org")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when organization has no URA but partOf points to org with URA",
		},
		{
			name: "two-level partOf chain: orgC -> orgB -> orgA (only orgA has URA)",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-c"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-b")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("org-b"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-a")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when partOf chain eventually leads to org with URA (2 levels)",
		},
		{
			name: "three-level partOf chain: orgD -> orgC -> orgB -> orgA (only orgA has URA)",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-d"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-c")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("org-b"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-a")},
					},
					{
						Id:     to.Ptr("org-c"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-b")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when partOf chain eventually leads to org with URA (3 levels)",
		},
		{
			name: "partOf chain ends without URA",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-c"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-b")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr("http://other-system.nl"), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("org-b"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-a")},
					},
				},
			},
			shouldSucceed: false,
			description:   "should fail when partOf chain ends at org without URA",
		},
		{
			name: "partOf references non-existent organization",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-c"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/non-existent")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when partOf references an organization not in the map",
		},
		{
			name: "circular reference in partOf chain: orgB -> orgC -> orgB",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-b"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-c")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("org-c"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-b")},
					},
				},
			},
			shouldSucceed: false,
			description:   "should fail when circular reference is detected in partOf chain",
		},
		{
			name: "partOf chain with no further references",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-c"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-b")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id: to.Ptr("org-b"),
						// No partOf and no URA
					},
				},
			},
			shouldSucceed: false,
			description:   "should fail when partOf chain ends at org with no URA and no further partOf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrganizationResource(tt.organization, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidateOrganizationResource_ComplexTrees(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"

	tests := []struct {
		name          string
		organization  *fhir.Organization
		parentOrgMap  map[*fhir.Organization][]*fhir.Organization
		shouldSucceed bool
		description   string
	}{
		{
			name: "complex tree: hospital with multiple departments",
			organization: &fhir.Organization{
				Id:     to.Ptr("cardiology-dept"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("emergency-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed for department in hospital tree structure",
		},
		{
			name: "complex tree: sub-department in department in hospital",
			organization: &fhir.Organization{
				Id:     to.Ptr("icu-cardiology"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed for sub-department in nested tree structure",
		},
		{
			name: "multiple parent orgs: org belongs to one tree",
			organization: &fhir.Organization{
				Id:     to.Ptr("clinic-dept"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/clinic-a")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
				}: {
					{
						Id:     to.Ptr("hospital-dept-a"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-a")},
					},
				},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
				}: {
					{
						Id:     to.Ptr("clinic-dept-b"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/clinic-a")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when org belongs to one of multiple parent organization trees",
		},
		{
			name: "organization with URA belonging to different parent tree",
			organization: &fhir.Organization{
				Id: to.Ptr("clinic-dept"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
				}: {},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when organization URA matches one of the parent trees",
		},
		{
			name: "deep nested tree (5 levels)",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-level-5"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-level-4")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-level-1"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("org-level-2"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-level-1")},
					},
					{
						Id:     to.Ptr("org-level-3"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-level-2")},
					},
					{
						Id:     to.Ptr("org-level-4"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-level-3")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed with deep nested tree (5 levels)",
		},
		{
			name: "organization partOf sibling (both under same parent)",
			organization: &fhir.Organization{
				Id:     to.Ptr("dept-b"),
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/dept-a")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("dept-a"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when org references sibling that has valid parent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrganizationResource(tt.organization, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidateOrganizationResource_EdgeCases(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"

	tests := []struct {
		name          string
		organization  *fhir.Organization
		parentOrgMap  map[*fhir.Organization][]*fhir.Organization
		shouldSucceed bool
		description   string
	}{
		{
			name:          "nil organization",
			organization:  nil,
			parentOrgMap:  map[*fhir.Organization][]*fhir.Organization{},
			shouldSucceed: true,
			description:   "should succeed (no validation) when organization is nil",
		},
		{
			name: "empty parent organization map",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
				},
			},
			parentOrgMap:  map[*fhir.Organization][]*fhir.Organization{},
			shouldSucceed: false,
			description:   "should fail when parent org map is empty and org has URA",
		},
		{
			name: "organization with empty URA value",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: nil},
				},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when organization has URA identifier with nil value",
		},
		{
			name: "partOf with empty reference",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-1"),
				PartOf: &fhir.Reference{Reference: to.Ptr("")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when partOf has empty reference string",
		},
		{
			name: "partOf with nil reference string",
			organization: &fhir.Organization{
				Id:     to.Ptr("org-1"),
				PartOf: &fhir.Reference{Reference: nil},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when partOf reference string is nil",
		},
		{
			name: "organization with both URA and valid partOf",
			organization: &fhir.Organization{
				Id: to.Ptr("org-1"),
				Identifier: []fhir.Identifier{
					{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
				},
				PartOf: &fhir.Reference{Reference: to.Ptr("Organization/parent-org")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("parent-org"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when organization has both valid URA and partOf (URA takes precedence)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrganizationResource(tt.organization, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidateLocationResource(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"
	ctx := t.Context()

	tests := []struct {
		name          string
		location      *fhir.Location
		parentOrgMap  map[*fhir.Organization][]*fhir.Organization
		shouldSucceed bool
		description   string
	}{
		// Single level org tree tests
		{
			name: "location with managingOrganization referencing parent org (single level)",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when location references parent org in single-level tree",
		},
		{
			name: "location with missing managingOrganization",
			location: &fhir.Location{
				Id: to.Ptr("location-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when location has no managingOrganization",
		},
		{
			name: "location referencing non-existent organization (single level)",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/non-existent")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when location references non-existent organization",
		},
		// One level org tree tests (parent + child)
		{
			name: "location referencing child org in one-level tree",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when location references child org in one-level tree",
		},
		{
			name: "location referencing parent org with child orgs present",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when location references parent org in one-level tree",
		},
		// Multi-level org tree tests
		{
			name: "location referencing grandchild org in multi-level tree",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/icu-cardiology")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("icu-cardiology"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when location references grandchild org in multi-level tree",
		},
		{
			name: "location in complex multi-level tree with multiple branches",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/er-triage")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("emergency-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("er-triage"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/emergency-dept")},
					},
					{
						Id:     to.Ptr("er-trauma"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/emergency-dept")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when location references org in complex multi-branch tree",
		},
		// Multiple parent orgs tests
		{
			name: "location referencing org from one of multiple parent trees",
			location: &fhir.Location{
				Id:                   to.Ptr("location-1"),
				ManagingOrganization: &fhir.Reference{Reference: to.Ptr("Organization/clinic-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
				}: {
					{
						Id:     to.Ptr("hospital-dept-a"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-a")},
					},
				},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
				}: {
					{
						Id:     to.Ptr("clinic-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/clinic-a")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when location references org from one of multiple parent trees",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLocationResource(ctx, tt.location, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidatePractitionerRoleResource(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"
	ctx := t.Context()

	tests := []struct {
		name             string
		practitionerRole *fhir.PractitionerRole
		parentOrgMap     map[*fhir.Organization][]*fhir.Organization
		shouldSucceed    bool
		description      string
	}{
		// Single level org tree tests
		{
			name: "practitioner role with organization referencing parent org (single level)",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when practitioner role references parent org in single-level tree",
		},
		{
			name: "practitioner role with missing organization",
			practitionerRole: &fhir.PractitionerRole{
				Id: to.Ptr("role-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when practitioner role has no organization reference",
		},
		{
			name: "practitioner role referencing non-existent organization (single level)",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/non-existent")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when practitioner role references non-existent organization",
		},
		// One level org tree tests (parent + child)
		{
			name: "practitioner role referencing child org in one-level tree",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when practitioner role references child org in one-level tree",
		},
		{
			name: "practitioner role referencing one of many children",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/surgery-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("radiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when practitioner role references one of many child orgs",
		},
		// Multi-level org tree tests
		{
			name: "practitioner role referencing deep nested org (3 levels)",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/icu-cardiology")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("icu-cardiology"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when practitioner role references deep nested org",
		},
		{
			name: "practitioner role in very deep tree (5 levels)",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/unit-level-5")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("org-level-1"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("unit-level-2"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/org-level-1")},
					},
					{
						Id:     to.Ptr("unit-level-3"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/unit-level-2")},
					},
					{
						Id:     to.Ptr("unit-level-4"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/unit-level-3")},
					},
					{
						Id:     to.Ptr("unit-level-5"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/unit-level-4")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when practitioner role references org in very deep tree (5 levels)",
		},
		// Multiple parent orgs tests
		{
			name: "practitioner role in multi-parent tree referencing second parent's child",
			practitionerRole: &fhir.PractitionerRole{
				Id:           to.Ptr("role-1"),
				Organization: &fhir.Reference{Reference: to.Ptr("Organization/clinic-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
				}: {
					{
						Id:     to.Ptr("hospital-dept-a"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-a")},
					},
				},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
				}: {
					{
						Id:     to.Ptr("clinic-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/clinic-a")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when practitioner role references org from second parent tree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePractitionerRoleResource(ctx, tt.practitionerRole, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidateHealthcareServiceResource(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"
	ctx := t.Context()

	tests := []struct {
		name              string
		healthcareService *fhir.HealthcareService
		parentOrgMap      map[*fhir.Organization][]*fhir.Organization
		shouldSucceed     bool
		description       string
	}{
		// Single level org tree tests
		{
			name: "healthcare service with providedBy referencing parent org (single level)",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when healthcare service references parent org in single-level tree",
		},
		{
			name: "healthcare service with missing providedBy",
			healthcareService: &fhir.HealthcareService{
				Id: to.Ptr("service-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when healthcare service has no providedBy reference",
		},
		{
			name: "healthcare service referencing non-existent organization (single level)",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/non-existent")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when healthcare service references non-existent organization",
		},
		// One level org tree tests (parent + child)
		{
			name: "healthcare service referencing child org in one-level tree",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when healthcare service references child org in one-level tree",
		},
		{
			name: "healthcare service provided by parent with multiple departments",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("surgery-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("radiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when healthcare service provided by parent with multiple departments",
		},
		// Multi-level org tree tests
		{
			name: "healthcare service referencing deep nested org (3 levels)",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/icu-cardiology")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("icu-cardiology"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when healthcare service references deep nested org",
		},
		{
			name: "healthcare service in complex multi-branch tree",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/er-trauma")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("emergency-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("er-triage"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/emergency-dept")},
					},
					{
						Id:     to.Ptr("er-trauma"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/emergency-dept")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when healthcare service references org in complex multi-branch tree",
		},
		// Multiple parent orgs tests
		{
			name: "healthcare service in multi-parent tree",
			healthcareService: &fhir.HealthcareService{
				Id:         to.Ptr("service-1"),
				ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/clinic-dept")},
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
				}: {
					{
						Id:     to.Ptr("hospital-dept-a"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-a")},
					},
				},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
				}: {
					{
						Id:     to.Ptr("clinic-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/clinic-a")},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when healthcare service references org from one of multiple parent trees",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHealthcareServiceResource(ctx, tt.healthcareService, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func TestValidateEndpointResource(t *testing.T) {
	uraSystem := "http://fhir.nl/fhir/NamingSystem/ura"
	ctx := t.Context()

	tests := []struct {
		name          string
		endpoint      *fhir.Endpoint
		parentOrgMap  map[*fhir.Organization][]*fhir.Organization
		shouldSucceed bool
		description   string
	}{
		// Single level org tree tests
		{
			name: "endpoint referenced by parent org (single level)",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-1")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced by parent org in single-level tree",
		},
		{
			name:     "endpoint with missing ID",
			endpoint: &fhir.Endpoint{
				// No ID
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when endpoint has no ID",
		},
		{
			name: "endpoint not referenced by any organization (single level)",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-orphan"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-1")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when endpoint is not referenced by any organization",
		},
		// One level org tree tests (parent + child)
		{
			name: "endpoint referenced by child org in one-level tree",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
						Endpoint: []fhir.Reference{
							{Reference: to.Ptr("Endpoint/endpoint-1")},
						},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced by child org in one-level tree",
		},
		{
			name: "endpoint referenced by multiple orgs in one-level tree",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-shared"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-shared")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
						Endpoint: []fhir.Reference{
							{Reference: to.Ptr("Endpoint/endpoint-shared")},
						},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced by multiple orgs",
		},
		{
			name: "endpoint among multiple endpoints in org",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-2"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-1")},
						{Reference: to.Ptr("Endpoint/endpoint-2")},
						{Reference: to.Ptr("Endpoint/endpoint-3")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is one of multiple endpoints in org",
		},
		// Multi-level org tree tests
		{
			name: "endpoint referenced by deep nested org (3 levels)",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("icu-cardiology"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/cardiology-dept")},
						Endpoint: []fhir.Reference{
							{Reference: to.Ptr("Endpoint/endpoint-1")},
						},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced by deep nested org",
		},
		{
			name: "endpoint in complex multi-branch tree",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-trauma"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
				}: {
					{
						Id:     to.Ptr("cardiology-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
						Endpoint: []fhir.Reference{
							{Reference: to.Ptr("Endpoint/endpoint-cardio")},
						},
					},
					{
						Id:     to.Ptr("emergency-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-main")},
					},
					{
						Id:     to.Ptr("er-trauma"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/emergency-dept")},
						Endpoint: []fhir.Reference{
							{Reference: to.Ptr("Endpoint/endpoint-trauma")},
						},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced by org in complex multi-branch tree",
		},
		{
			name: "endpoint with absolute URL reference",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-1"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-main"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("12345")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("http://example.org/fhir/Endpoint/endpoint-1")},
					},
				}: {},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced with absolute URL",
		},
		// Multiple parent orgs tests
		{
			name: "endpoint in multi-parent tree referenced by second parent's child",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-clinic"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-hospital")},
					},
				}: {
					{
						Id:     to.Ptr("hospital-dept-a"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/hospital-a")},
					},
				},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
				}: {
					{
						Id:     to.Ptr("clinic-dept"),
						PartOf: &fhir.Reference{Reference: to.Ptr("Organization/clinic-a")},
						Endpoint: []fhir.Reference{
							{Reference: to.Ptr("Endpoint/endpoint-clinic")},
						},
					},
				},
			},
			shouldSucceed: true,
			description:   "should succeed when endpoint is referenced by org from one of multiple parent trees",
		},
		{
			name: "endpoint not referenced in multi-parent tree",
			endpoint: &fhir.Endpoint{
				Id: to.Ptr("endpoint-orphan"),
			},
			parentOrgMap: map[*fhir.Organization][]*fhir.Organization{
				{
					Id: to.Ptr("hospital-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("11111")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-hospital")},
					},
				}: {},
				{
					Id: to.Ptr("clinic-a"),
					Identifier: []fhir.Identifier{
						{System: to.Ptr(uraSystem), Value: to.Ptr("22222")},
					},
					Endpoint: []fhir.Reference{
						{Reference: to.Ptr("Endpoint/endpoint-clinic")},
					},
				}: {},
			},
			shouldSucceed: false,
			description:   "should fail when endpoint is not referenced by any org in multi-parent tree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEndpointResource(ctx, tt.endpoint, tt.parentOrgMap)

			if tt.shouldSucceed {
				require.NoError(t, err, tt.description)
			} else {
				require.Error(t, err, tt.description)
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
