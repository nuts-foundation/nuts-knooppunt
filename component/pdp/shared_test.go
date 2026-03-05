package pdp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubjectProperties_UnmarshalJSON(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		const data = `{"client_qualifications": ["mcsd_update"], "subject_organization_id": "00000666", "other": "value"}`
		var actual SubjectProperties
		err := json.Unmarshal([]byte(data), &actual)
		require.NoError(t, err)

		expected := SubjectProperties{
			OtherProps: map[string]any{
				"other": "value",
			},
			ClientQualifications:  []string{"mcsd_update"},
			SubjectOrganizationId: "00000666",
		}
		require.Equal(t, expected, actual)
	})
	t.Run("marshal", func(t *testing.T) {
		subjectProps := SubjectProperties{
			OtherProps: map[string]any{
				"other": "value",
			},
			ClientId:              "1",
			SubjectId:             "2",
			SubjectOrganization:   "3",
			SubjectFacilityType:   "Z3",
			SubjectRole:           "GP",
			ClientQualifications:  []string{"mcsd_update"},
			SubjectOrganizationId: "00000666",
		}
		data, err := json.Marshal(subjectProps)
		require.NoError(t, err)
		assert.JSONEq(t, `{"other":"value", "client_id":"1","client_qualifications":["mcsd_update"],"subject_id":"2","subject_organization_id":"00000666","subject_organization":"3","subject_facility_type":"Z3","subject_role":"GP"}`, string(data))
	})
}
