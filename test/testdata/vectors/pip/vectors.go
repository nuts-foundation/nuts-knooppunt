package pip

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func HAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "policy-information-point",
		ID:   8,
	}
}

func CapabilityStatements() []fhir.CapabilityStatement {
	return []fhir.CapabilityStatement{
		{
			Id:      to.Ptr("05671E0B-9029-4DFD-79F3-FB583CCCE4C7"),
			Version: to.Ptr("0.0.1"),
			Name:    to.Ptr("MCSDUpdateCapabilityStatement"),
			Title:   to.Ptr("Capability Statement for exposing the MCSD admin directory"),
			Status:  fhir.PublicationStatusActive,
			Date:    "2025-11-19",
			Kind:    fhir.CapabilityStatementKindInstance,
			Format:  []string{"json"},
			Rest: []fhir.CapabilityStatementRest{
				{
					Mode: fhir.RestfulCapabilityModeServer,
					Resource: []fhir.CapabilityStatementRestResource{
						{
							Type: fhir.ResourceTypeOrganization,
							Interaction: []fhir.CapabilityStatementRestResourceInteraction{
								{
									Code: fhir.TypeRestfulInteractionRead,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryInstance,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryType,
								},
							},
						},
						{
							Type: fhir.ResourceTypeHealthcareService,
							Interaction: []fhir.CapabilityStatementRestResourceInteraction{
								{
									Code: fhir.TypeRestfulInteractionRead,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryInstance,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryType,
								},
							},
						},
						{
							Type: fhir.ResourceTypeLocation,
							Interaction: []fhir.CapabilityStatementRestResourceInteraction{
								{
									Code: fhir.TypeRestfulInteractionRead,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryInstance,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryType,
								},
							},
						},
						{
							Type: fhir.ResourceTypeEndpoint,
							Interaction: []fhir.CapabilityStatementRestResourceInteraction{
								{
									Code: fhir.TypeRestfulInteractionRead,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryInstance,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryType,
								},
							},
						},
						{
							Type: fhir.ResourceTypePractitionerRole,
							Interaction: []fhir.CapabilityStatementRestResourceInteraction{
								{
									Code: fhir.TypeRestfulInteractionRead,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryInstance,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryType,
								},
							},
						},
						{
							Type: fhir.ResourceTypeOrganizationAffiliation,
							Interaction: []fhir.CapabilityStatementRestResourceInteraction{
								{
									Code: fhir.TypeRestfulInteractionRead,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryInstance,
								},
								{
									Code: fhir.TypeRestfulInteractionHistoryType,
								},
							},
						},
					},
				},
			},
		},
	}
}

func Resources() []fhir.HasId {
	var resources []fhir.HasId
	for _, capability := range CapabilityStatements() {
		resources = append(resources, to.Ptr(capability))
	}
	return resources
}
