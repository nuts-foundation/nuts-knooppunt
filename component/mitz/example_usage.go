package mitz

// Example demonstrates how to use CreateXACMLAuthzDecisionQuery
func Example() string {
	// Create request matching the values from example.xml
	req := XACMLRequest{
		// Resource attributes
		PatientBSN:             "900186021",
		HealthcareFacilityType: "Z3",
		AuthorInstitutionID:    "00000659",

		// Action attributes
		EventCode: "GGC002",

		// Subject attributes
		SubjectRole:            "01.015",
		ProviderID:             "000095254",
		ProviderInstitutionID:  "00000666",
		ConsultingFacilityType: "Z3",

		// Environment attributes
		PurposeOfUse: "TREAT",

		// Endpoint
		ToAddress: "http://localhost:8000/4",
	}

	xml, err := CreateXACMLAuthzDecisionQuery(req)
	if err != nil {
		panic(err)
	}

	return xml
}
