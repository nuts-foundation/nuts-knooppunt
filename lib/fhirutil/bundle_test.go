package fhirutil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestVisitBundleResources(t *testing.T) {
	// Create a sample Patient resource
	patient := fhir.Patient{}
	patientData, err := json.Marshal(patient)
	require.NoError(t, err)

	// Create a bundle with one entry
	bundle := &fhir.Bundle{
		Entry: []fhir.BundleEntry{{
			Resource: patientData,
		}},
	}
	called := false

	err = VisitBundleResources[fhir.Patient](bundle, func(res *fhir.Patient) error {
		called = true
		res.Id = to.Ptr("test-patient")
		return nil
	})

	require.NoError(t, err)
	require.True(t, called, "visitor function was not called")
	require.Equal(t, "{\"id\":\"test-patient\",\"resourceType\":\"Patient\"}", string(bundle.Entry[0].Resource))
}
