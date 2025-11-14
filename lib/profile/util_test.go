package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestSet(t *testing.T) {
	t.Run("no meta", func(t *testing.T) {
		// Test when meta is nil - should create new meta with the profile
		result := Set(nil, NLGenericFunctionOrganization)

		assert.NotNil(t, result)
		assert.Len(t, result.Profile, 1)
		assert.Equal(t, NLGenericFunctionOrganization, result.Profile[0])
	})

	t.Run("with other profiles", func(t *testing.T) {
		// Test when meta already has other profiles - should add the new profile
		existingProfile := "http://example.com/existing-profile"
		meta := &fhir.Meta{
			Profile: []string{existingProfile},
		}

		result := Set(meta, NLGenericFunctionEndpoint)

		assert.Len(t, result.Profile, 2)
		assert.Contains(t, result.Profile, existingProfile)
		assert.Contains(t, result.Profile, NLGenericFunctionEndpoint)
		assert.Equal(t, existingProfile, result.Profile[0])
		assert.Equal(t, NLGenericFunctionEndpoint, result.Profile[1])
	})

	t.Run("profile exists", func(t *testing.T) {
		// Test when the profile already exists - should not add duplicate
		meta := &fhir.Meta{
			Profile: []string{NLGenericFunctionLocation, "http://example.com/other-profile"},
		}

		result := Set(meta, NLGenericFunctionLocation)

		assert.Len(t, result.Profile, 2)
		assert.Contains(t, result.Profile, NLGenericFunctionLocation)
		assert.Contains(t, result.Profile, "http://example.com/other-profile")
		// Should not have duplicates
		profileCount := 0
		for _, profile := range result.Profile {
			if profile == NLGenericFunctionLocation {
				profileCount++
			}
		}
		assert.Equal(t, 1, profileCount)
	})
}
