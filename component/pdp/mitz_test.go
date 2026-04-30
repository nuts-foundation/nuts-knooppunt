package pdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseMitzInput() PolicyInput {
	return PolicyInput{
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
}

func TestComponent_map_input_xacml(t *testing.T) {
	req, err := xacmlFromInput(baseMitzInput())
	require.NoError(t, err)
	assert.Equal(t, "900186021", req.PatientBSN)
	assert.Equal(t, "01.015", req.SubjectRole)
	assert.Equal(t, "000095254", req.ProviderID)
	assert.Equal(t, "00000666", req.ProviderInstitutionID)
	assert.Equal(t, "Z3", req.ConsultingFacilityType)
	assert.Equal(t, "00000659", req.AuthorInstitutionID)
	assert.Equal(t, "Z3", req.HealthcareFacilityType)
	assert.Equal(t, "TREAT", req.PurposeOfUse)
	assert.Nil(t, req.MandatedID, "MandatedID should not be set when no delegation is present")
}

func TestComponent_xacmlFromInput_Delegation(t *testing.T) {
	const (
		delegatorID   = "000012345"
		delegatorRole = "30.000"
		regByToken    = "http://fhir.nl/fhir/NamingSystem/uzi-nr-pers|" + delegatorID
		roleCodeToken = "http://fhir.nl/fhir/NamingSystem/uzi-rolcode" + delegatorRole
	)

	t.Run("uses delegating practitioner as responsible and sets MandatedID to authenticated user", func(t *testing.T) {
		input := baseMitzInput()
		input.Subject.OtherProps = OtherProps{
			"delegation_registered_by": regByToken,
			"delegation_role_code":     roleCodeToken,
		}

		req, err := xacmlFromInput(input)
		require.NoError(t, err)
		assert.Equal(t, delegatorID, req.ProviderID, "ProviderID should be the practitioner who delegated")
		assert.Equal(t, delegatorRole, req.SubjectRole, "SubjectRole should be the delegated role")
		require.NotNil(t, req.MandatedID, "MandatedID should be set on a delegated request")
		assert.Equal(t, "000095254", *req.MandatedID, "MandatedID should be the authenticated user's id")
	})

	t.Run("returns error when delegation_role_code is missing", func(t *testing.T) {
		input := baseMitzInput()
		input.Subject.OtherProps = OtherProps{
			"delegation_registered_by": regByToken,
		}

		_, err := xacmlFromInput(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing delegation_role_code")
	})

	t.Run("returns error when delegation_registered_by is empty", func(t *testing.T) {
		input := baseMitzInput()
		input.Subject.OtherProps = OtherProps{
			"delegation_registered_by": "",
			"delegation_role_code":     roleCodeToken,
		}

		_, err := xacmlFromInput(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format for delegation_registered_by")
	})

	t.Run("returns error when delegation_registered_by has no system|value separator", func(t *testing.T) {
		input := baseMitzInput()
		input.Subject.OtherProps = OtherProps{
			"delegation_registered_by": "no-pipe-here",
			"delegation_role_code":     roleCodeToken,
		}

		_, err := xacmlFromInput(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format for delegation_registered_by")
	})

	t.Run("returns error when delegation_registered_by has only system, no value", func(t *testing.T) {
		input := baseMitzInput()
		input.Subject.OtherProps = OtherProps{
			"delegation_registered_by": "http://fhir.nl/fhir/NamingSystem/uzi-nr-pers|",
			"delegation_role_code":     roleCodeToken,
		}

		_, err := xacmlFromInput(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format for delegation_registered_by")
	})

	t.Run("returns error when delegation_role_code is malformed", func(t *testing.T) {
		input := baseMitzInput()
		input.Subject.OtherProps = OtherProps{
			"delegation_registered_by": regByToken,
			"delegation_role_code":     "garbage",
		}

		_, err := xacmlFromInput(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delegation_role_code")
	})
}
