package pdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_map_input_xacml(t *testing.T) {
	input := MainPolicyInput{
		Method:                           "GET",
		Path:                             []string{"fhir", "Patient", "118876"},
		ResourceId:                       "118876",
		ResourceType:                     fhir.ResourceTypePatient,
		PatientBSN:                       "900186021",
		RequestingUziRoleCode:            "01.015",
		RequestingPractitionerIdentifier: "000095254",
		RequestingOrganizationUra:        "00000666",
		RequestingFacilityType:           "Z3",
		DataHolderOrganizationUra:        "00000659",
		DataHolderFacilityType:           "Z3",
		PurposeOfUse:                     "treatment",
	}

	xacml := xacmlFromInput(input)
	assert.Equal(t, "900186021", xacml.PatientBSN)
	assert.Equal(t, "01.015", xacml.SubjectRole)
	assert.Equal(t, "000095254", xacml.ProviderID)
	assert.Equal(t, "00000666", xacml.ProviderInstitutionID)
	assert.Equal(t, "Z3", xacml.ConsultingFacilityType)
	assert.Equal(t, "00000659", xacml.AuthorInstitutionID)
	assert.Equal(t, "Z3", xacml.HealthcareFacilityType)
	assert.Equal(t, "TREAT", xacml.PurposeOfUse)
}
