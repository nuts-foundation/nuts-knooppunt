package mcsd

import (
	"net/url"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Test_mapResourceIDResolver_resolve(t *testing.T) {
	t.Run("known entry", func(t *testing.T) {
		resolver := mapResourceIDResolver{
			"Patient/123": "local-1",
		}
		localID, err := resolver.resolve(nil, "Patient/123")
		require.NoError(t, err)
		require.Equal(t, "local-1", *localID)
	})
	t.Run("unknown entry", func(t *testing.T) {
		resolver := mapResourceIDResolver{}
		localID, err := resolver.resolve(nil, "Patient/123")
		require.NoError(t, err)
		require.Nil(t, localID)
	})
}

func Test_metaSourceResourceIDResolver_resolve(t *testing.T) {
	baseURL, _ := url.Parse("http://example.com/")
	t.Run("known entry", func(t *testing.T) {
		fhirClient := &test.StubFHIRClient{
			Resources: []any{
				&fhir.Patient{
					Id: to.Ptr("local-1"),
					Meta: &fhir.Meta{
						Source: to.Ptr(baseURL.JoinPath("Patient/123").String()),
					},
				},
			},
		}
		resolver := metaSourceResourceIDResolver{
			sourceFHIRBaseURL: baseURL,
			localFHIRClient:   fhirClient,
		}
		resourceID, err := resolver.resolve(t.Context(), "Patient/123")
		require.NoError(t, err)
		require.Equal(t, "local-1", *resourceID)
	})
	t.Run("unknown entry", func(t *testing.T) {
		fhirClient := &test.StubFHIRClient{}
		resolver := metaSourceResourceIDResolver{
			sourceFHIRBaseURL: baseURL,
			localFHIRClient:   fhirClient,
		}
		resourceID, err := resolver.resolve(t.Context(), "Patient/123")
		require.NoError(t, err)
		require.Nil(t, resourceID)
	})
	t.Run("multiple resources match", func(t *testing.T) {
		fhirClient := &test.StubFHIRClient{
			Resources: []any{
				&fhir.Patient{
					Id: to.Ptr("local-1"),
					Meta: &fhir.Meta{
						Source: to.Ptr(baseURL.JoinPath("Patient/123").String()),
					},
				},
				&fhir.Patient{
					Id: to.Ptr("local-2"),
					Meta: &fhir.Meta{
						Source: to.Ptr(baseURL.JoinPath("Patient/123").String()),
					},
				},
			},
		}
		resolver := metaSourceResourceIDResolver{
			sourceFHIRBaseURL: baseURL,
			localFHIRClient:   fhirClient,
		}
		_, err := resolver.resolve(t.Context(), "Patient/123")
		require.EqualError(t, err, "resource id resolution: multiple resources found for Patient/123")
	})
	t.Run("FHIR client error", func(t *testing.T) {
		fhirClient := &test.StubFHIRClient{
			Error: assert.AnError,
		}
		resolver := metaSourceResourceIDResolver{
			sourceFHIRBaseURL: baseURL,
			localFHIRClient:   fhirClient,
		}
		_, err := resolver.resolve(t.Context(), "Patient/123")
		require.ErrorIs(t, err, assert.AnError)
	})
}
