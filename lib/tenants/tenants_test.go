package tenants

import (
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestID_UnmarshalText(t *testing.T) {
	t.Run("empty value", func(t *testing.T) {
		var id ID
		err := id.UnmarshalText([]byte(""))
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header", fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("invalid token", func(t *testing.T) {
		var id ID
		err := id.UnmarshalText([]byte("something"))
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header", fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("invalid system", func(t *testing.T) {
		var id ID
		err := id.UnmarshalText([]byte("something|1"))
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header, expected system: "+coding.URANamingSystem, fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("no value", func(t *testing.T) {
		var id ID
		err := id.UnmarshalText([]byte(coding.URANamingSystem + "|"))
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header, missing value", fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("valid", func(t *testing.T) {
		var id ID
		err := id.UnmarshalText([]byte(coding.URANamingSystem + "|1"))
		require.NoError(t, err)
		require.Equal(t, "1", id.URA())
	})
}
