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
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/bsn"),
					Code:   to.Ptr("12345"),
				},
			},
		},
		Custodian: &fhir.Reference{
			Type: to.Ptr("Organization"),
			Identifier: &fhir.Identifier{
				System: to.Ptr(coding.URANamingSystem),
				Value:  to.Ptr("00000030"),
			},
		},
	}
	requestHeaders := fhirclient.RequestHeaders(map[string][]string{
		"X-Tenant-ID": {coding.URANamingSystem + "|00000030"},
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

		t.Run("search through NVI Gateway with patient:identifier", func(t *testing.T) {
			var searchSet fhir.Bundle
			err := nviGatewayClient.Search("DocumentReference", url.Values{
				"patient:identifier": []string{"http://fhir.nl/fhir/NamingSystem/bsn|12345"},
			}, &searchSet, requestHeaders)
			require.NoError(t, err)
			require.Len(t, searchSet.Entry, 1)
		})
	})
}
