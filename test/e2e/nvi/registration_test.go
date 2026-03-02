package nvi

import (
	"encoding/json"
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

const (
	bsnSystem         = "http://fhir.nl/fhir/NamingSystem/bsn"
	bsnValue          = "12345"
	sourceIdentSystem = "https://cp1-test.example.org/device-identifiers"
	sourceIdentValue  = "EHR-SYS-2024-001"
	tenantURA         = "00000030"
)

func newRegistrationBundle(t *testing.T) fhir.Bundle {
	t.Helper()
	listResource := fhir.List{
		Status: fhir.ListStatusCurrent,
		Mode:   fhir.ListModeWorking,
		Extension: []fhir.Extension{
			{
				Url: "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian",
				ValueReference: &fhir.Reference{
					Identifier: &fhir.Identifier{
						System: to.Ptr(coding.URANamingSystem),
						Value:  to.Ptr(tenantURA),
					},
				},
			},
		},
		Subject: &fhir.Reference{
			Identifier: &fhir.Identifier{
				System: to.Ptr(bsnSystem),
				Value:  to.Ptr(bsnValue),
			},
		},
		Source: &fhir.Reference{
			Type: to.Ptr("Device"),
			Identifier: &fhir.Identifier{
				System: to.Ptr(sourceIdentSystem),
				Value:  to.Ptr(sourceIdentValue),
			},
		},
		Code: &fhir.CodeableConcept{
			Coding: []fhir.Coding{
				{
					System:  to.Ptr("http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-zorgcontext-cs"),
					Code:    to.Ptr("MEDAFSPRAAK"),
					Display: to.Ptr("Medicatieafspraak"),
				},
			},
		},
	}
	listJSON, err := json.Marshal(listResource)
	require.NoError(t, err)
	return fhir.Bundle{
		Type: fhir.BundleTypeTransaction,
		Entry: []fhir.BundleEntry{
			{
				Request: &fhir.BundleEntryRequest{
					Method: fhir.HTTPVerbPOST,
					Url:    "List",
				},
				Resource: listJSON,
			},
		},
	}
}

func Test_Registration(t *testing.T) {
	harnessDetail := harness.Start(t)

	requestHeaders := fhirclient.RequestHeaders(map[string][]string{
		"X-Tenant-ID": {coding.URANamingSystem + "|" + tenantURA},
	})
	nviGatewayClient := fhirclient.New(harnessDetail.KnooppuntInternalBaseURL.JoinPath("nvi"), http.DefaultClient, nil)

	// Register first List via Bundle transaction
	var result1 fhir.Bundle
	err := nviGatewayClient.CreateWithContext(t.Context(), newRegistrationBundle(t), &result1, fhirclient.AtPath("/"), requestHeaders)
	require.NoError(t, err)

	// Register a second one to ensure multiple registrations work
	var result2 fhir.Bundle
	err = nviGatewayClient.CreateWithContext(t.Context(), newRegistrationBundle(t), &result2, fhirclient.AtPath("/"), requestHeaders)
	require.NoError(t, err)

	t.Run("search by patient.identifier", func(t *testing.T) {
		var searchSet fhir.Bundle
		err := nviGatewayClient.Search("List", url.Values{
			"patient.identifier": []string{bsnSystem + "|" + bsnValue},
		}, &searchSet, requestHeaders)
		require.NoError(t, err)
		require.Len(t, searchSet.Entry, 2)
	})

	t.Run("search by subject.identifier", func(t *testing.T) {
		var searchSet fhir.Bundle
		err := nviGatewayClient.Search("List", url.Values{
			"subject.identifier": []string{bsnSystem + "|" + bsnValue},
		}, &searchSet, requestHeaders)
		require.NoError(t, err)
		require.Len(t, searchSet.Entry, 2)
	})

	t.Run("search by source.identifier", func(t *testing.T) {
		var searchSet fhir.Bundle
		err := nviGatewayClient.Search("List", url.Values{
			"source.identifier": []string{sourceIdentSystem + "|" + sourceIdentValue},
		}, &searchSet, requestHeaders)
		require.NoError(t, err)
		require.Len(t, searchSet.Entry, 2)
	})

	t.Run("search without identifier returns error", func(t *testing.T) {
		var searchSet fhir.Bundle
		err := nviGatewayClient.Search("List", url.Values{
			"status": []string{"current"},
		}, &searchSet, requestHeaders)
		require.Error(t, err)
	})
}
