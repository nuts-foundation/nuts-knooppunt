package zonnebloem

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// NewHandler is how gf-sandbox mounts the EHR. The clients it lists come from
// the store at the URL it was given, path included: that store is the only
// thing the EHR talks to.
func TestNewHandler_ListsTheClientsOfTheGivenStore(t *testing.T) {
	var searched string
	fhirStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searched = r.URL.Path
		writeFHIR(t, w, http.StatusOK, searchSet(fhir.Patient{
			Id:   to.Ptr("pool-anna-zonnebloem-patient"),
			Name: []fhir.HumanName{{Given: []string{"Anna"}, Family: to.Ptr("Jansen")}},
		}))
	}))
	t.Cleanup(fhirStore.Close)
	storeURL, err := url.Parse(fhirStore.URL + "/fhir/sunflower-patients")
	require.NoError(t, err)
	ehr := httptest.NewServer(NewHandler(storeURL))
	t.Cleanup(ehr.Close)

	status, body := get(t, ehr.URL+"/")

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "/fhir/sunflower-patients/Patient/_search", searched)
	require.Contains(t, body, `href="/clients/pool-anna-zonnebloem-patient"`)
}
