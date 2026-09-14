package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceRequest is one call the fake source received.
type sourceRequest struct {
	Method string
	Path   string
	Query  url.Values
	Form   url.Values
	Auth   string
}

// all returns every value the request carried, wherever it carried it, so a test
// can assert that something is absent from the request as a whole.
func (r sourceRequest) all() string { return r.Path + "?" + r.Query.Encode() + "&" + r.Form.Encode() }

// fakeSource stands in for De Zonnebloem's PEP. routes is keyed by FHIR resource
// type; anything not in it answers 404, so a query the chain should not have
// issued shows up as a failed outcome rather than passing silently.
func fakeSource(t *testing.T, got *[]sourceRequest, routes map[string]string) sourceAddress {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = append(*got, sourceRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
			Form: r.PostForm, Auth: r.Header.Get("Authorization"),
		})
		resourceType := strings.TrimSuffix(r.URL.Path[len("/fhir/"):], "/_search")
		body, ok := routes[resourceType]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}
}

func bundleOf(resources ...string) string {
	body := `{"resourceType":"Bundle","type":"searchset","entry":[`
	for i, resource := range resources {
		if i > 0 {
			body += ","
		}
		body += `{"resource":` + resource + `}`
	}
	return body + `]}`
}

const zonnebloemPatient = `{"resourceType":"Patient","id":"zb-anna",
  "identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"999900006"}],
  "name":[{"given":["Anna"],"family":"Jansen"}]}`

const metoprolol = `{"resourceType":"MedicationRequest","id":"zb-metoprolol","status":"active","intent":"order",
  "medicationCodeableConcept":{"text":"Metoprolol","coding":[{"system":"http://snomed.info/sct","code":"372826007","display":"Metoprolol"}]},
  "dosageInstruction":[{"text":"Metoprolol 50 mg 1dd"}]}`

const penicillinAllergy = `{"resourceType":"AllergyIntolerance","id":"zb-allergy",
  "code":{"text":"Penicilline","coding":[{"system":"http://snomed.info/sct","code":"373270004","display":"Penicilline"}]},
  "recordedDate":"2019"}`

const diabetes = `{"resourceType":"Condition","id":"zb-dm2",
  "code":{"text":"Diabetes mellitus type 2","coding":[{"system":"http://snomed.info/sct","code":"44054006"}]},
  "onsetDateTime":"2012"}`

// The chain must issue exactly the searches the BGZ policy authorizes: the
// Patient search scoped by BSN identifier with the general-practitioner include,
// and then one search per localized category, scoped to the local Patient id the
// first call resolved.
func TestRetrieveBGZ_IssuesTheAuthorizedQueries(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":            bundleOf(zonnebloemPatient),
		"MedicationRequest":  bundleOf(metoprolol),
		"AllergyIntolerance": bundleOf(penicillinAllergy),
		"Condition":          bundleOf(diabetes),
	})
	categories := []string{nvi.CategoryAllergyIntolerance, nvi.CategoryCondition,
		nvi.CategoryMedicationRequest, nvi.CategoryPatient}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", categories)

	require.NoError(t, err)
	require.Len(t, got, 4, "one Patient search plus one per retrievable category")

	// A POST _search, not a GET. The BSN is the one identifier on this path that
	// must not reach a URL: the PEP logs $request, so a query-string BSN lands in
	// an access log outside anything this application can clear.
	patientSearch := got[0]
	assert.Equal(t, http.MethodPost, patientSearch.Method)
	assert.Equal(t, "/fhir/Patient/_search", patientSearch.Path)
	assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/bsn|999900006", patientSearch.Form.Get("identifier"))
	assert.Equal(t, "Patient:general-practitioner", patientSearch.Form.Get("_include"))
	assert.NotContains(t, patientSearch.Query.Encode(), "999900006", "no BSN in the URL")
	assert.Equal(t, "Bearer the-token", patientSearch.Auth, "the PEP rejects anything else")

	byPath := map[string]sourceRequest{}
	for _, request := range got[1:] {
		byPath[request.Path] = request
		assert.Equal(t, "Patient/zb-anna", request.Query.Get("patient"),
			"every follow-up search is scoped to the id the Patient search resolved")
		assert.Equal(t, "Bearer the-token", request.Auth)
	}
	assert.Contains(t, byPath, "/fhir/AllergyIntolerance")
	assert.Contains(t, byPath, "/fhir/Condition")
	// The one query the BGZ policy constrains beyond the patient scope. Without
	// both of these the policy denies it.
	assert.Equal(t, "http://snomed.info/sct|16076005",
		byPath["/fhir/MedicationRequest"].Query.Get("category"))
	assert.Equal(t, "MedicationRequest:medication",
		byPath["/fhir/MedicationRequest"].Query.Get("_include"))

	assert.Equal(t, chainGranted, result.Outcome)
	for _, outcome := range result.Queries {
		assert.Equal(t, http.StatusOK, outcome.Status, outcome.Label)
	}
}

// The NVI says which categories a source holds. Asking for one it never
// registered is a request the demo has no grounds for, and would show an empty
// section as though the source had nothing rather than as never asked.
func TestRetrieveBGZ_AsksOnlyForLocalizedCategories(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":   bundleOf(zonnebloemPatient),
		"Condition": bundleOf(diabetes),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "/fhir/Condition", got[1].Path)
	assert.Equal(t, chainGranted, result.Outcome)
}

// A denial at the source must leave the screen with nothing to render. Carrying
// on would expose data from the queries that happened to pass while the chain as
// a whole was refused.
func TestRetrieveBGZ_ADeniedPatientSearchStopsTheChain(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, sourceRequest{Path: r.URL.Path})
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	source.Address = server.URL + "/fhir"

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition, nvi.CategoryMedicationRequest})

	require.NoError(t, err, "a denial is an outcome to render, not a transport failure")
	assert.Equal(t, chainRefused, result.Outcome)
	assert.Empty(t, result.Items, "a denied chain exposes no data")
	require.Len(t, got, 1, "nothing is asked after the source refuses")
	require.Len(t, result.Queries, 1)
	assert.Equal(t, http.StatusForbidden, result.Queries[0].Status)
}

// A source that holds no Patient for this BSN cannot be queried further: every
// remaining BGZ search is scoped to a local Patient id that does not exist.
//
// Access was still granted, and the screen has to say so rather than present an
// empty result as a refusal: the two have different causes and different fixes.
func TestRetrieveBGZ_NoPatientAtTheSourceIsNotADenial(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{"Patient": bundleOf()})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Equal(t, chainGranted, result.Outcome, "the source answered and served the request; it just held no match")
	assert.Empty(t, result.PatientID)
	assert.Empty(t, result.Items)
	require.Len(t, got, 1, "no follow-up search can be scoped without a local patient id")
	assert.Contains(t, result.Queries[0].Error, "no patient")
}

// Every retrieved element has to name where it came from, which is the whole
// point of the enriched record.
func TestRetrieveBGZ_AttributesEveryItemToTheSource(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":            bundleOf(zonnebloemPatient),
		"AllergyIntolerance": bundleOf(penicillinAllergy),
		"MedicationRequest":  bundleOf(metoprolol),
		"Condition":          bundleOf(diabetes),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryAllergyIntolerance, nvi.CategoryCondition,
			nvi.CategoryMedicationRequest, nvi.CategoryPatient})

	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	sections := map[string]recordItem{}
	for _, item := range result.Items {
		assert.Equal(t, "Zorgcentrum De Zonnebloem", item.Source)
		sections[item.Section] = item
	}
	assert.Equal(t, "Penicilline", sections["Allergies"].Title)
	assert.Equal(t, "Metoprolol", sections["Medication"].Title)
	assert.Equal(t, "Diabetes mellitus type 2", sections["Conditions"].Title)
	assert.Equal(t, "Metoprolol 50 mg 1dd", sections["Medication"].Detail)
}

// The marker path: a note carrying the DEMO- convention is what proves the data
// travelled from the other system rather than being local all along.
func TestRetrieveBGZ_FlagsTheDemoMarker(t *testing.T) {
	marked := `{"resourceType":"AllergyIntolerance","id":"zb-marked",
	  "code":{"text":"Pinda"},
	  "note":[{"text":"Vastgesteld na reactie. DEMO-BLUE-BUTTERFLY-42"}]}`
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":            bundleOf(zonnebloemPatient),
		"AllergyIntolerance": bundleOf(marked),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryAllergyIntolerance})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.True(t, result.Items[0].Marker, "an entry with a DEMO- note is the marker proof")
	assert.Contains(t, result.Items[0].Detail, "DEMO-BLUE-BUTTERFLY-42")
}

// answering serves one canned status and body to every request.
func answering(t *testing.T, got *[]sourceRequest, status int, body string) sourceAddress {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = append(*got, sourceRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Form: r.PostForm})
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}
}

// A source that breaks did not refuse. Rendering a 502 as a denial tells the
// presenter the source said no, which it never did, and sends them looking at
// consent and credentials instead of at the source being down.
func TestRetrieveBGZ_AServerErrorIsAFailureNotARefusal(t *testing.T) {
	var got []sourceRequest
	source := answering(t, &got, http.StatusBadGateway, "")

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Equal(t, chainFailed, result.Outcome)
	assert.Empty(t, result.Items)
	require.Len(t, got, 1, "nothing is asked after the first call fails")
}

func TestRetrieveBGZ_RateLimitingIsAFailureNotARefusal(t *testing.T) {
	var got []sourceRequest
	source := answering(t, &got, http.StatusTooManyRequests, "")

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient})

	require.NoError(t, err)
	assert.Equal(t, chainFailed, result.Outcome)
}

// An unreachable source establishes nothing at all.
func TestRetrieveBGZ_AnUnreachableSourceIsAFailure(t *testing.T) {
	var got []sourceRequest
	source := answering(t, &got, http.StatusOK, "")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	source.Address = server.URL + "/fhir"
	server.Close() // nothing is listening now

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient})

	require.NoError(t, err, "an unreachable source is an outcome to render")
	assert.Equal(t, chainFailed, result.Outcome)
	require.Len(t, result.Queries, 1)
	assert.NotEmpty(t, result.Queries[0].Error)
}

// A 200 carrying something that is not a Bundle means the answer could not be
// read. It is not evidence about what the source holds, and the screen must not
// turn it into one: the old behaviour reported "access granted" and then
// overwrote the parse error with a claim that the source holds no such patient.
func TestRetrieveBGZ_AnUnreadableBodyIsAFailureAndKeepsItsReason(t *testing.T) {
	var got []sourceRequest
	source := answering(t, &got, http.StatusOK, "not json at all")

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Equal(t, chainFailed, result.Outcome)
	assert.Empty(t, result.Items)
	require.Len(t, result.Queries, 1)
	assert.Contains(t, result.Queries[0].Error, "Bundle")
	assert.NotContains(t, result.Queries[0].Error, "no patient",
		"a body we could not read says nothing about which patients the source has")
}

// The request line the authorization screen renders is kept in memory and shown
// on a projector. It must not carry the BSN either.
func TestRetrieveBGZ_TheRenderedRequestCarriesNoBSN(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":   bundleOf(zonnebloemPatient),
		"Condition": bundleOf(diabetes),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.NotEmpty(t, result.Queries)
	for _, outcome := range result.Queries {
		assert.NotContains(t, outcome.Request, "999900006", outcome.Label)
		assert.NotContains(t, outcome.Error, "999900006", outcome.Label)
	}
	// It still has to say what was asked, or the screen loses its point.
	assert.Contains(t, result.Queries[0].Request, "Patient/_search")
}

// FHIR servers page search results. Reading only the first page and saying
// nothing renders a partial record as a complete one, which on a clinical
// overview is the worst kind of wrong.
func TestRetrieveBGZ_FollowsPagingAndReportsWhatItCannotFinish(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		pages++
		next := ""
		if pages < 3 {
			next = `,"link":[{"relation":"next","url":"` + "http://" + r.Host + `/fhir/Condition?page=` + string(rune('0'+pages)) + `"}]`
		}
		body := `{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` + diabetes + `}]` + next + `}`
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Equal(t, 3, pages, "every page the source offered is fetched")
	assert.Len(t, result.Items, 3, "and every page's entries reach the record")
}

// A source that keeps offering pages must not be followed forever, and the
// screen has to say the result was cut short rather than present it as whole.
func TestRetrieveBGZ_BoundsPagingAndSaysSoWhenItStops(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` + diabetes +
			`}],"link":[{"relation":"next","url":"http://` + r.Host + `/fhir/Condition?more=1"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	var conditions queryOutcome
	for _, outcome := range result.Queries {
		if outcome.Label == "Conditions" {
			conditions = outcome
		}
	}
	assert.Contains(t, conditions.Error, "incomplete",
		"a truncated result must say so rather than look complete")
}

// FHIR allows a MedicationRequest to name its medication by reference instead of
// inline, which is why the search asks for _include. Reading only the inline
// form silently drops the medication from the record.
func TestRetrieveBGZ_ResolvesAnIncludedMedicationReference(t *testing.T) {
	const byReference = `{"resourceType":"MedicationRequest","id":"zb-ref","status":"active","intent":"order",
	  "medicationReference":{"reference":"Medication/med-1"},
	  "dosageInstruction":[{"text":"Metoprolol 50 mg 1dd"}]}`
	const included = `{"resourceType":"Medication","id":"med-1",
	  "code":{"text":"Metoprolol","coding":[{"system":"http://snomed.info/sct","code":"372826007"}]}}`
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":           bundleOf(zonnebloemPatient),
		"MedicationRequest": bundleOf(byReference, included),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryMedicationRequest})

	require.NoError(t, err)
	require.Len(t, result.Items, 1, "the medication must reach the record")
	assert.Equal(t, "Metoprolol", result.Items[0].Title)
	assert.Equal(t, "Metoprolol 50 mg 1dd", result.Items[0].Detail)
}
