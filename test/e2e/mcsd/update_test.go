package mcsd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Test_mCSDUpdateClient(t *testing.T) {
	harnessDetail := harness.Start(t)
	t.Run("Force update mCSD Client", func(t *testing.T) {
		httpResponse, err := http.Post(harnessDetail.KnooppuntInternalBaseURL.JoinPath("mcsd/update").String(), "application/json", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, httpResponse.StatusCode)
		responseData, err := io.ReadAll(httpResponse.Body)
		require.NoError(t, err)
		println(string(responseData))

		var response mcsd.UpdateReport
		require.NoError(t, json.Unmarshal(responseData, &response))
		assert.Equalf(t, 4, mapEntrySuffix(response, "lrza-mcsd-admin").CountCreated, "created=4 in %v", response)

		cacheFHIRClient := fhirclient.New(harnessDetail.MCSDCacheFHIRBaseURL, http.DefaultClient, nil)
		t.Run("assert Sunflower organization resources", func(t *testing.T) {
			org, err := searchOrg(cacheFHIRClient, harnessDetail.SunflowerURA)
			require.NoError(t, err)
			assert.Equal(t, "Sunflower Care Home", *org.Name)
		})
		t.Run("assert Care2Cure organization resources", func(t *testing.T) {
			org, err := searchOrg(cacheFHIRClient, harnessDetail.Care2CureURA)
			require.NoError(t, err)
			assert.Equal(t, "Care2Cure Hospital", *org.Name)
		})
	})
}

func searchOrg(client fhirclient.Client, ura string) (*fhir.Organization, error) {
	var searchResult fhir.Bundle
	err := client.Search("Organization", url.Values{"identifier": []string{coding.URANamingSystem + "|" + ura}}, &searchResult)
	if err != nil {
		return nil, err
	}
	if len(searchResult.Entry) == 0 {
		return nil, nil
	}
	var organization fhir.Organization
	if err := json.Unmarshal(searchResult.Entry[0].Resource, &organization); err != nil {
		return nil, err
	}
	return &organization, nil
}

func mapEntrySuffix(r mcsd.UpdateReport, suffix string) mcsd.DirectoryUpdateReport {
	for key, value := range r {
		if strings.HasSuffix(key, suffix) {
			return value
		}
	}
	return mcsd.DirectoryUpdateReport{}
}
