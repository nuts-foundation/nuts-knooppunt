package pdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComponent_map_input_xacml(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			User: PolicySubjectUser{
				Id:   "000095254",
				Role: "01.015",
			},
			Organization: PolicySubjectOrganization{
				Ura:          "00000666",
				FacilityType: "Z3",
			},
		},
		Context: PolicyContext{
			PatientBSN:               "900186021",
			DataHolderFacilityType:   "Z3",
			DataHolderOrganizationId: "00000659",
		},
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
