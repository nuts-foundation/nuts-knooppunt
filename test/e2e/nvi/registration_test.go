package nvi

import (
	"net/http"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Test_Registration(t *testing.T) {
	harnessDetail := harness.Start(t)

	expected := fhir.DocumentReference{
		Status: fhir.DocumentReferenceStatusCurrent,
		Type: &fhir.CodeableConcept{
			Coding: []fhir.Coding{
				{
					System: to.Ptr("system"),
					Code:   to.Ptr("code"),
				},
			},
		},
	}
	requestHeaders := fhirclient.RequestHeaders(map[string][]string{
		"X-Tenant-ID": {coding.URANamingSystem + "|1"},
	})
	nviGatewayClient := fhirclient.New(harnessDetail.KnooppuntInternalBaseURL.JoinPath("nvi"), http.DefaultClient, nil)

	var actual fhir.DocumentReference
	err := nviGatewayClient.Create(expected, &actual, requestHeaders)
	require.NoError(t, err)

	// Create a second one to ensure multiple registrations work
	var second fhir.DocumentReference
	err = nviGatewayClient.Create(expected, &second, requestHeaders)
	require.NoError(t, err)

	t.Run("assert created document reference", func(t *testing.T) {
		require.NotNil(t, actual.Id)
		require.Equal(t, expected.Status, actual.Status)

		t.Run("search through NVI Gateway with POST", func(t *testing.T) {
			var searchSet fhir.Bundle
			err := nviGatewayClient.Search("DocumentReference", url.Values{
				"_id": []string{*actual.Id},
			}, &searchSet, requestHeaders)
			require.NoError(t, err)
			require.Len(t, searchSet.Entry, 1)
		})
		t.Run("search through NVI Gateway with GET", func(t *testing.T) {
			nviGatewayClient := fhirclient.New(harnessDetail.KnooppuntInternalBaseURL.JoinPath("nvi"), http.DefaultClient, &fhirclient.Config{
				UsePostSearch: false,
			})
			var searchSet fhir.Bundle
			err := nviGatewayClient.Search("DocumentReference", url.Values{"_id": []string{*actual.Id}}, &searchSet, requestHeaders)
			require.NoError(t, err)
			require.Len(t, searchSet.Entry, 1)
		})
		t.Run("read DocumentReference directly from NVI", func(t *testing.T) {
			nviClient := fhirclient.New(harnessDetail.Vectors.NVI.FHIRBaseURL, http.DefaultClient, nil)
			var fetched fhir.DocumentReference
			err = nviClient.Read("DocumentReference/"+*actual.Id, &fetched, requestHeaders)
			require.NoError(t, err)
			require.Equal(t, expected.Status, fetched.Status)
			require.Equal(t, expected.Type.Coding[0].System, fetched.Type.Coding[0].System)
			require.Equal(t, expected.Type.Coding[0].Code, fetched.Type.Coding[0].Code)
		})
	})
}
