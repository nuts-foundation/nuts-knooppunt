package nvi

import (
	"net/http"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
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
	nviGatewayClient := fhirclient.New(harnessDetail.KnooppuntInternalBaseURL.JoinPath("nvi"), http.DefaultClient, nil)

	var actual fhir.DocumentReference
	err := nviGatewayClient.Create(expected, &actual)
	require.NoError(t, err)

	t.Run("assert created document reference", func(t *testing.T) {
		require.NotNil(t, actual.Id)
		require.Equal(t, expected.Status, actual.Status)

		t.Run("read DocumentReference directly from NVI", func(t *testing.T) {
			nviClient := fhirclient.New(harnessDetail.Vectors.NVI.FHIRBaseURL, http.DefaultClient, nil)
			var fetched fhir.DocumentReference
			err = nviClient.Read("DocumentReference/"+*actual.Id, &fetched)
			require.NoError(t, err)
			require.Equal(t, expected.Status, fetched.Status)
			require.Equal(t, expected.Type.Coding[0].System, fetched.Type.Coding[0].System)
			require.Equal(t, expected.Type.Coding[0].Code, fetched.Type.Coding[0].Code)
		})
	})
}
