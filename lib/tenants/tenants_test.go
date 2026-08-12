package tenants

import (
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestIDFromHeaderValue(t *testing.T) {
	t.Run("missing header", func(t *testing.T) {
		_, err := IDFromHeaderValue("")
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "missing tenant request header: X-Tenant-ID", fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("invalid token", func(t *testing.T) {
		_, err := IDFromHeaderValue("something")
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header", fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("invalid system", func(t *testing.T) {
		_, err := IDFromHeaderValue("something|1")
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header, expected system: "+coding.URANamingSystem, fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("no value", func(t *testing.T) {
		_, err := IDFromHeaderValue(coding.URANamingSystem + "|")
		fhirError := &fhirapi.Error{}
		require.ErrorAs(t, err, &fhirError)
		require.Equal(t, "invalid tenant ID in request header, missing value", fhirError.Message)
		require.Equal(t, fhir.IssueTypeValue, fhirError.IssueType)
	})
	t.Run("valid", func(t *testing.T) {
		identifier, err := IDFromHeaderValue(coding.URANamingSystem + "|1")
		require.NoError(t, err)
		require.Equal(t, &fhir.Identifier{
			System: to.Ptr(coding.URANamingSystem),
			Value:  to.Ptr("1"),
		}, identifier)
	})
}
