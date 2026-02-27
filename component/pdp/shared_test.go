package pdp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubjectProperties_UnmarshalJSON(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		const data = `{"scope": "mcsd_update mscd_query", "organization_ura": "00000666", "other": "value"}`
		var actual PDPSubject
		err := json.Unmarshal([]byte(data), &actual)
		require.NoError(t, err)

		expected := PDPSubject{
			OtherProps: map[string]any{
				"other": "value",
			},
			Scope:           "mcsd_update mscd_query",
			OrganizationUra: "00000666",
		}
		require.Equal(t, expected, actual)
	})
	t.Run("marshal", func(t *testing.T) {
		subjectProps := PDPSubject{
			OtherProps: map[string]any{
				"other": "value",
			},
			ClientId:                 "1",
			UserId:                   "2",
			UserRole:                 "GP",
			OrganizationUra:          "00000666",
			OrganizationName:         "3",
			OrganizationFacilityType: "Z3",
			Scope:                    "mcsd_update",
		}
		data, err := json.Marshal(subjectProps)
		require.NoError(t, err)
		assert.JSONEq(t, `{"other":"value", "client_id":"1","scope":"mcsd_update","user_id":"2","organization_ura":"00000666","organization_name":"3","organization_facility_type":"Z3","user_role":"GP","active":false}`, string(data))
	})
}
