package pip

import (
	"net/url"

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

func Patients() []fhir.Patient {
	return []fhir.Patient{
		{
			Id: to.Ptr("3E439979-017F-40AA-594D-EBCF880FFD97"),
			Identifier: []fhir.Identifier{
				{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/bsn"),
					Value:  to.Ptr("176286603"),
				},
			},
		},
	}
}

func Organizations() []fhir.Organization {
	return []fhir.Organization{
		{
			Id: to.Ptr("7DC623BA-0EF1-42AD-0AAD-F4D034F67C9F"),
			Identifier: []fhir.Identifier{
				{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
					Value:  to.Ptr("00000030"),
				},
			},
		},
		{
			Id: to.Ptr("873236BC-73E9-4AF2-20FB-A4CA28CA3CC7"),
			Identifier: []fhir.Identifier{
				{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
					Value:  to.Ptr("00000040"),
				},
			},
		},
	}
}

func Observations() []fhir.Observation {
	return []fhir.Observation{
		{
			Id: to.Ptr("7DC623BA-0EF1-42AD-0AAD-F4D034F67C9F"),
		},
	}
}

func Compositions() []fhir.Composition {
	return []fhir.Composition{
		{
			Id: to.Ptr("21ef0423-018b-40e7-adfd-7f4317f01c8f"),
		},
	}
}

func Task() []fhir.Task {
	return []fhir.Task{
		{
			Id: to.Ptr("12AF22F3-2DE5-47E1-B3CB-B053C8621F84"),
		},
	}
}

func Consents() []fhir.Consent {
	return []fhir.Consent{
		{
			Id:     to.Ptr("D3D29954-2559-4226-FD45-3C6C3632C5C4"),
			Status: fhir.ConsentStateActive,
			Scope: fhir.CodeableConcept{
				Coding: []fhir.Coding{
					{
						System: to.Ptr("http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-consent-scope"),
						Code:   to.Ptr("eoverdracht"),
					},
				},
			},
			Organization: []fhir.Reference{
				{
					Identifier: to.Ptr(fhir.Identifier{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("00000030"),
					}),
				},
			},
			Provision: to.Ptr(fhir.ConsentProvision{
				Type: to.Ptr(fhir.ConsentProvisionTypePermit),
				Actor: []fhir.ConsentProvisionActor{
					{
						Reference: fhir.Reference{
							Identifier: to.Ptr(fhir.Identifier{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("00000040"),
							}),
						},
					},
				},
				Data: []fhir.ConsentProvisionData{
					{
						Reference: fhir.Reference{
							Reference: to.Ptr("Observation/7DC623BA-0EF1-42AD-0AAD-F4D034F67C9F"),
							Type:      to.Ptr("Observation"),
						},
					},
					{
						Reference: fhir.Reference{
							Reference: to.Ptr("Composition/21ef0423-018b-40e7-adfd-7f4317f01c8f"),
							Type:      to.Ptr("Composition"),
						},
					},
					{
						Reference: fhir.Reference{
							Reference: to.Ptr("Task/12AF22F3-2DE5-47E1-B3CB-B053C8621F84"),
							Type:      to.Ptr("Task"),
						},
					},
				},
				Action: []fhir.CodeableConcept{
					{
						Coding: []fhir.Coding{
							{
								System: to.Ptr("http://terminology.hl7.org/CodeSystem/consentaction"),
								Code:   to.Ptr("access"),
							},
						},
					},
				},
			}),
		},
	}
}

func Resources(fhirBaseURL *url.URL) []fhir.HasId {
	var resources []fhir.HasId
	for _, patient := range Patients() {
		resources = append(resources, &patient)
	}
	for _, org := range Organizations() {
		resources = append(resources, &org)
	}
	for _, org := range Observations() {
		resources = append(resources, &org)
	}
	for _, curr := range Compositions() {
		resources = append(resources, &curr)
	}
	for _, curr := range Task() {
		resources = append(resources, &curr)
	}
	for _, consent := range Consents() {
		resources = append(resources, &consent)
	}
	return resources
}
