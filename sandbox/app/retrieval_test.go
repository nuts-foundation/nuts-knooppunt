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
		if got != nil {
			*got = append(*got, sourceRequest{
				Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
				Form: r.PostForm, Auth: r.Header.Get("Authorization"),
			})
		}
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
	assert.Contains(t, result.Queries[0].Note, "no patient")
	assert.Empty(t, result.Queries[0].Error, "an empty result is not an error")
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
// on a projector. It must not carry the BSN either, and neither must the error
// text beside it: Go's HTTP client puts the whole request URL in the error it
// returns, so a failing query is where a leaked identifier would surface. One
// query here is therefore made to fail at transport level, because asserting
// that an empty string holds no BSN proves nothing at all.
func TestRetrieveBGZ_TheRenderedRequestAndItsErrorCarryNoBSN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		// Drop the connection mid-response, which is what a source going away
		// looks like from here and what populates Error with the request URL.
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Queries, 2)
	require.NotEmpty(t, result.Queries[1].Error,
		"the failing query has to have produced error text for this test to check anything")
	for _, outcome := range result.Queries {
		assert.NotContains(t, outcome.Request, "999900006", outcome.Label)
		assert.NotContains(t, outcome.Error, "999900006", outcome.Label)
		assert.NotContains(t, outcome.Truncated, "999900006", outcome.Label)
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
	assert.Contains(t, conditions.Truncated, "incomplete",
		"a truncated result must say so rather than look complete")
	assert.Empty(t, conditions.Error, "being cut short is not the same as failing")
	// Stopping keeps what was already read; a truncation reported as an error
	// would discard it.
	assert.NotEmpty(t, result.Items, "the pages already read are kept")
	assert.True(t, result.Incomplete())
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

// A next link is a URL the source chose. Following it with the access token
// attached lets any source redirect that token to an origin of its choosing, and
// one request is enough to lose it: bounding the number of pages bounds requests,
// not exposure.
func TestRetrieveBGZ_NeverSendsTheTokenToAnotherOrigin(t *testing.T) {
	var elsewhereSawToken bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhereSawToken = r.Header.Get("Authorization") != ""
		_, _ = w.Write([]byte(bundleOf(diabetes)))
	}))
	t.Cleanup(elsewhere.Close)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` + diabetes +
			`}],"link":[{"relation":"next","url":"` + elsewhere.URL + `/fhir/Condition?page=2"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.False(t, elsewhereSawToken, "the access token must never reach another origin")
	assert.Len(t, result.Items, 1, "the page we did read is still shown")
	assert.True(t, result.Incomplete(), "and the screen has to say the result was cut short")
}

// A page that fails must not discard the pages already read. Dropping them turns
// a partial answer into no answer, which is a bigger loss than the truncation.
func TestRetrieveBGZ_KeepsThePagesItAlreadyRead(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		pages++
		if pages > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` + diabetes +
			`}],"link":[{"relation":"next","url":"http://` + r.Host + `/fhir/Condition?page=2"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Len(t, result.Items, 1, "the first page survives a failure on the second")
	assert.True(t, result.Incomplete())
}

// FHIR allows a reference to be absolute, matched against the entry's fullUrl.
func TestRetrieveBGZ_ResolvesAnAbsoluteMedicationReference(t *testing.T) {
	const byAbsoluteRef = `{"resourceType":"MedicationRequest","id":"zb-ref","status":"active","intent":"order",
	  "medicationReference":{"reference":"http://zonnebloem.example/fhir/Medication/med-1"},
	  "dosageInstruction":[{"text":"Metoprolol 50 mg 1dd"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[
		  {"resource":` + byAbsoluteRef + `},
		  {"fullUrl":"http://zonnebloem.example/fhir/Medication/med-1",
		   "resource":{"resourceType":"Medication","id":"med-1","code":{"text":"Metoprolol"}}}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryMedicationRequest})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "Metoprolol", result.Items[0].Title)
}

// Decoding without error is not the same as being a search result. null, an
// empty object, an OperationOutcome and a Bundle of another type all unmarshal
// happily, and each was reported as a served request that found no patient.
func TestRetrieveBGZ_RequiresASearchsetBundle(t *testing.T) {
	for _, body := range []string{
		`null`,
		`{}`,
		`{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"processing"}]}`,
		`{"resourceType":"Bundle","type":"collection","entry":[]}`,
	} {
		t.Run(body[:min(len(body), 28)], func(t *testing.T) {
			var got []sourceRequest
			source := answering(t, &got, http.StatusOK, body)

			result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
				[]string{nvi.CategoryPatient})

			require.NoError(t, err)
			require.Equal(t, http.StatusOK, result.Queries[0].Status,
				"the source served this body; recognising it is what has to fail")
			assert.Equal(t, chainFailed, result.Outcome)
			assert.Contains(t, result.Queries[0].Error, "searchset",
				"and the reason has to name the envelope, so this stays a decoder test")
			assert.NotContains(t, result.Queries[0].Error, "no patient",
				"a response we could not recognise says nothing about the source's records")
		})
	}
}

// The verdict is about authorization, which the Patient search did establish.
// But a screen that says access was granted, narrates four passed checks and
// offers a link to the data, while every clinical query failed, overstates what
// happened.
func TestRetrieveBGZ_SaysSoWhenTheClinicalSearchesFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition, nvi.CategoryAllergyIntolerance})

	require.NoError(t, err)
	assert.Equal(t, chainGranted, result.Outcome, "the source did authorize the request")
	assert.Empty(t, result.Items)
	assert.True(t, result.Incomplete(), "but nothing it authorized could be retrieved")
}

// A source that answered in full and simply holds no such patient retrieved
// everything there was. Reporting that as "not everything came back" invents a
// problem, and the note explaining the empty result is what caused it: it was
// being written into Error, which classifies failures.
func TestRetrieveBGZ_AnEmptyResultIsCompleteNotIncomplete(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{"Patient": bundleOf()})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Equal(t, chainGranted, result.Outcome)
	assert.False(t, result.Incomplete(), "nothing failed and nothing was cut short")
	require.Len(t, result.Queries, 1)
	assert.Empty(t, result.Queries[0].Error, "an empty result is not an error")
	assert.Contains(t, result.Queries[0].Note, "no patient", "but the screen still has to explain it")
}

// The origin check must not reject the same origin written differently. RFC 6454
// makes a default port and the absent port equivalent, and the host
// case-insensitive, so being stricter than that drops pages for no benefit.
func TestSameOrigin_NormalisesPortAndCase(t *testing.T) {
	for _, tc := range []struct {
		base, candidate string
		same            bool
	}{
		{"https://source.example/fhir", "https://source.example:443/fhir?page=2", true},
		{"https://source.example:443/fhir", "https://source.example/fhir?page=2", true},
		{"http://source.example/fhir", "http://SOURCE.example:80/fhir?page=2", true},
		{"http://source.example:8080/fhir", "http://source.example:8080/fhir?page=2", true},
		{"https://source.example/fhir", "http://source.example/fhir?page=2", false},
		{"https://source.example/fhir", "https://elsewhere.example/fhir?page=2", false},
		{"https://source.example/fhir", "https://source.example:8443/fhir?page=2", false},
	} {
		t.Run(tc.base+" vs "+tc.candidate, func(t *testing.T) {
			base, err := url.Parse(tc.base)
			require.NoError(t, err)
			assert.Equal(t, tc.same, sameOrigin(base, tc.candidate))
		})
	}
}

// Both CodeableConcept.text and Coding.display are optional in FHIR R4
// (https://hl7.org/fhir/R4/datatypes-definitions.html#CodeableConcept.text), so
// a source is entitled to answer with the code alone. Rendering nothing for such
// a resource removed a diagnosis from a clinical overview without saying so,
// which is worse than an ugly line: the screen looked complete.
func TestRetrieveBGZ_ShowsAResourceCarryingOnlyACode(t *testing.T) {
	codeOnly := `{"resourceType":"Condition","id":"zb-coded",
	  "code":{"coding":[{"system":"http://snomed.info/sct","code":"44054006"}]},
	  "onsetDateTime":"2012"}`
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":   bundleOf(zonnebloemPatient),
		"Condition": bundleOf(codeOnly),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Items, 1, "the condition must survive, coded or not")
	assert.Contains(t, result.Items[0].Title, "44054006",
		"and must carry the code, which is the only identification the source gave")
	assert.False(t, result.Incomplete(),
		"nothing was dropped, so the retrieval is not partial")
}

// The other half: a resource this build genuinely cannot render is dropped, and
// dropping it silently is the thing to avoid. The record then holds less than
// the source sent while every query reads as a clean 200.
func TestRetrieveBGZ_ReportsResourcesItCouldNotRender(t *testing.T) {
	unreadable := `{"resourceType":"Condition","id":"zb-broken","code":{"coding":"not-an-array"}}`
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":   bundleOf(zonnebloemPatient),
		"Condition": bundleOf(diabetes, unreadable),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Items, 1, "the readable one is still shown")
	assert.True(t, result.Incomplete(),
		"and the screen has to say the overview is not everything the source sent")
}

// The BSN is kept out of request URLs by sending it in the body of the Patient
// search, but the id that search returns is the source's to choose, and FHIR
// permits any id syntax (https://hl7.org/fhir/R4/datatypes.html#id). A source
// that identifies its patients by BSN would put it straight back into the query
// string of every clinical search, and from there into the PEP's access log and
// onto the authorization screen. The chain has to notice rather than comply.
func TestRetrieveBGZ_RefusesToScopeOnAnIDThatCarriesTheBSN(t *testing.T) {
	selfIdentifying := `{"resourceType":"Patient","id":"999900006",
	  "identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"999900006"}]}`
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":   bundleOf(selfIdentifying),
		"Condition": bundleOf(diabetes),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, got, 1, "the Patient search was answered; nothing after it may be sent")
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the source served the search; its answer is what this refuses to act on")
	require.Contains(t, result.Queries[0].Truncated, "containing the BSN",
		"and the reason has to be the identifier rather than any other stop")
	for _, request := range got {
		assert.NotContains(t, request.Path+"?"+request.Query.Encode(), "999900006",
			"no request line may carry the BSN")
	}
	assert.Empty(t, result.Items, "the clinical searches were not sent, so nothing came back")
	assert.True(t, result.Incomplete(),
		"and the screen has to say why the overview is empty rather than imply the source holds nothing")
}

// The Patient search is where the chain decides whether the source knows this
// patient at all, and that answer stops everything else. Reading one page and
// treating "no Patient here" as "no Patient anywhere" turns an unfinished search
// into an absence claim, which is the one thing this screen must never invent.
func TestRetrieveBGZ_DoesNotClaimAbsenceFromAnUnfinishedPatientSearch(t *testing.T) {
	practitioner := `{"resourceType":"Practitioner","id":"zb-gp","name":[{"family":"Bakker"}]}`
	var patientPages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fhir/Patient") {
			_, _ = w.Write([]byte(bundleOf(diabetes)))
			return
		}
		patientPages++
		if patientPages == 1 {
			// A first page carrying only the _include companion, and a link
			// saying the match is further on.
			_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
				practitioner + `}],"link":[{"relation":"next","url":"http://` + r.Host + `/fhir/Patient?page=2"}]}`))
			return
		}
		_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	assert.Equal(t, "zb-anna", result.PatientID, "the match on the second page is the patient")
	assert.NotContains(t, result.Queries[0].Note, "holds no patient",
		"the source does hold this patient; it just did not fit on one page")
	assert.NotEmpty(t, result.Items, "and the clinical searches ran on that id")
}

// The same search, cut short for real: the continuation cannot be read. Saying
// the source holds nobody would be an answer this chain never got.
func TestRetrieveBGZ_SaysThePatientSearchWasCutShortRatherThanEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[],` +
			`"link":[{"relation":"next","url":"http://` + r.Host + `/fhir/Patient?page=2"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the first page was served; the continuation after it is what failed")
	require.Contains(t, result.Queries[0].Truncated, "later page",
		"and the reason has to be the page that could not be read")
	assert.NotContains(t, result.Queries[0].Note, "holds no patient",
		"an unread continuation is not an answer that the patient is unknown here")
	assert.True(t, result.Incomplete(), "and the screen has to say the search did not finish")
}

// Two reasons for an incomplete result can hold at once: a page that could not
// be fetched and a resource that arrived but could not be rendered. Writing the
// second over the first told the reader about one line while an unknown number
// of further resources was never asked for at all.
func TestRetrieveBGZ_KeepsEveryReasonAResultIsIncomplete(t *testing.T) {
	const nameless = `{"resourceType":"Condition","id":"zb-nameless","code":{"coding":[{}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/fhir/Patient"):
			_, _ = w.Write([]byte(bundleOf(zonnebloemPatient)))
		case r.URL.Query().Get("page") == "2":
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
				diabetes + `},{"resource":` + nameless + `}],"link":[{"relation":"next","url":"http://` +
				r.Host + `/fhir/Condition?page=2"}]}`))
		}
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Queries, 2)
	assert.Contains(t, result.Queries[1].Truncated, "later page",
		"the page that could not be read has to survive")
	assert.Contains(t, result.Queries[1].Truncated, "could not be rendered",
		"and so does the resource that arrived but could not be shown")
}

// A continuation link is a URL the source composed, and a server that echoes the
// search into it puts the BSN in a request line: the same leak the POST body was
// chosen to avoid, reintroduced by following the link. The page is worth less
// than the identifier, so the chain stops and says the result is incomplete.
func TestRetrieveBGZ_RefusesAContinuationCarryingTheBSN(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
			zonnebloemPatient + `}],"link":[{"relation":"next","url":"http://` + r.Host +
			`/fhir/Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|999900006&page=2"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", []string{nvi.CategoryPatient})

	require.NoError(t, err)
	for _, path := range paths {
		assert.NotContains(t, path, "999900006", "no request line may carry the BSN")
	}
	require.NotEmpty(t, result.Queries)
	assert.NotEmpty(t, result.Queries[0].Truncated, "and refusing the page leaves the result incomplete")
	assert.NotContains(t, result.Queries[0].Truncated, "999900006",
		"the reason must not repeat what it refused to send")
}

// An entry the source meant as a match but this chain cannot read is not an
// empty answer. Reporting "holds no patient with this BSN" over it states
// something the search never established, and stops the chain on that basis.
func TestRetrieveBGZ_DoesNotCallAnUnreadableMatchAnAbsence(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient": `{"resourceType":"Bundle","type":"searchset","entry":[` +
			`{"resource":{"resourceType":"Patient","id":123}}]}`,
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", []string{nvi.CategoryPatient})

	require.NoError(t, err)
	require.NotEmpty(t, result.Queries)
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the source served this answer; the entry in it is what could not be read")
	assert.Empty(t, result.Queries[0].Note, "an unreadable match is not an answer that the patient is absent")
	assert.True(t, result.Incomplete(), "and the result has to say it fell short")
}

// A source holding two records for one BSN has an identity question this
// application cannot settle. Taking the first entry picks one person's clinical
// record by the order the server happened to serve it in, and shows it as theirs.
func TestRetrieveBGZ_StopsWhenTheBSNMatchesTwoPatients(t *testing.T) {
	const second = `{"resourceType":"Patient","id":"zb-anna-2",
	  "identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/bsn","value":"999900006"}]}`
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient":   bundleOf(zonnebloemPatient, second),
		"Condition": bundleOf(diabetes),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Queries, 1, "nothing may be asked about a patient that was not identified")
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the source served this answer; holding two patients is what stops the chain")
	require.Empty(t, result.Queries[0].Error)
	require.Contains(t, result.Queries[0].Truncated, "2 patients with this BSN",
		"and the reason has to be the ambiguity rather than any other stop")
	assert.True(t, result.Incomplete())
	assert.Empty(t, result.Items)
}

// Percent-encoding a digit changes the bytes in the URL and not what the URL
// says: RFC 3986 section 2.3 makes %39 and 9 the same character. A check on the
// raw link therefore reads as safe a URL that carries the BSN into the source's
// access log the moment it is requested.
func TestRetrieveBGZ_RefusesAContinuationCarryingAnEncodedBSN(t *testing.T) {
	const encoded = "%39%39%39%39%30%30%30%30%36"
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
			zonnebloemPatient + `}],"link":[{"relation":"next","url":"http://` + r.Host +
			`/fhir/Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|` + encoded + `"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", []string{nvi.CategoryPatient})

	require.NoError(t, err)
	// One request, the initial search. Counting matters: following the link until
	// the page cap also leaves a nonempty Truncated, so a test that only checks
	// for a reason would pass while every one of those pages was sent.
	require.Len(t, paths, 1, "the continuation may not be requested at all")
	require.NotEmpty(t, result.Queries)
	require.Contains(t, result.Queries[0].Truncated, "patient identifier in its URL",
		"and the reason has to be the identifier, not the page cap")
}

// One entry decoded and another did not. The one that did not may be a second
// patient, so how many the source matched is unknown, and picking the readable
// one presents a possibly-wrong person's record as this patient's.
func TestRetrieveBGZ_StopsWhenOnlySomeMatchesCouldBeRead(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{
		"Patient": `{"resourceType":"Bundle","type":"searchset","entry":[` +
			`{"resource":` + zonnebloemPatient + `},` +
			`{"resource":{"resourceType":"Patient","id":123}}]}`,
		"Condition": bundleOf(diabetes),
	})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Queries, 1, "nothing may be asked about a patient that was not identified")
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the source served this answer; an entry in it is what could not be read")
	require.Empty(t, result.Queries[0].Error)
	require.Contains(t, result.Queries[0].Truncated, "could not be read")
	require.Empty(t, result.Items)
}

// A URL can carry the identifier somewhere a path-and-query check never looks.
// Go decodes userinfo while parsing and keeps the username when it formats a
// transport error, so a link nobody inspected there puts the plain BSN into the
// text this application stores and renders. Credentials in a paging link are not
// a thing this chain has any use for, so the shape is refused rather than the
// spelling chased.
func TestRetrieveBGZ_RefusesAContinuationWithCredentials(t *testing.T) {
	const encoded = "%39%39%39%39%30%30%30%30%36"
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
			zonnebloemPatient + `}],"link":[{"relation":"next","url":"http://` + encoded + `@` +
			r.Host + `/fhir/Patient?page=2"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", []string{nvi.CategoryPatient})

	require.NoError(t, err)
	require.Len(t, paths, 1, "the continuation may not be requested at all")
	require.NotEmpty(t, result.Queries)
	// Without these two the test also passes when the first request never
	// succeeds: one recorded path, an incomplete result, and no identifier
	// anywhere, established by nothing.
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the first search was served; the link it offered is what this refuses")
	require.Contains(t, result.Queries[0].Truncated, "carrying credentials",
		"and the reason has to be the credentials rather than any other stop")
	for _, outcome := range result.Queries {
		for _, text := range []string{outcome.Truncated, outcome.Error, outcome.Request} {
			assert.NotContains(t, text, "999900006", "neither spelling may be retained")
			assert.NotContains(t, text, encoded)
		}
	}
	assert.True(t, result.Incomplete(), "and refusing the page leaves the result incomplete")
}

// Refusing a link this build cannot read is right; saying it carried the patient
// identifier is a claim about a URL nobody could parse. The screen reports what
// was established, which is that the link was unusable.
func TestRetrieveBGZ_SaysAnUnreadableContinuationIsUnreadable(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
			zonnebloemPatient + `}],"link":[{"relation":"next","url":"http://` + r.Host +
			`/fhir/Patient?cursor=%ZZ"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", []string{nvi.CategoryPatient})

	require.NoError(t, err)
	require.Len(t, paths, 1, "the link is refused before it is requested, not after it fails")
	require.NotEmpty(t, result.Queries)
	assert.NotContains(t, result.Queries[0].Truncated, "patient identifier",
		"nothing established that this link carried an identifier")
	// The whole guard reason, because "could not be read" on its own also matches
	// the message for a page that was fetched and failed.
	assert.Contains(t, result.Queries[0].Truncated, "continuation whose URL could not be read",
		"what did happen is that the link was unusable")
}

// The request line is read off a projector. For the searches that travel in the
// body, the placeholder standing in for the value is not a value, so encoding it
// as one turns "[sent in the request body]" into %5Bsent+in+the+request+body%5D
// and the reader learns nothing from a row that exists to be read.
func TestRetrieveBGZ_RendersTheBodyPlaceholderReadably(t *testing.T) {
	var got []sourceRequest
	source := fakeSource(t, &got, map[string]string{"Patient": bundleOf(zonnebloemPatient)})

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006", []string{nvi.CategoryPatient})

	require.NoError(t, err)
	require.NotEmpty(t, result.Queries)
	assert.Contains(t, result.Queries[0].Request, "identifier=[sent in the request body]")
	assert.NotContains(t, result.Queries[0].Request, "%5B", "the placeholder is prose, not a value to encode")
}

// One readable match on a page that had a continuation is not one match. A
// second record for this BSN can sit on the page that was never read, and
// committing to the first turns "the source answered partially" into "this is
// the patient", which is the selection by entry order the ambiguity stop exists
// to refuse.
func TestRetrieveBGZ_DoesNotCommitToAPatientFromAnUnfinishedSearch(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[{"resource":` +
			zonnebloemPatient + `}],"link":[{"relation":"next","url":"http://` + r.Host +
			`/fhir/Patient?page=2"}]}`))
	}))
	t.Cleanup(server.Close)
	source := sourceAddress{URA: "00000020", Name: "Zorgcentrum De Zonnebloem", Address: server.URL + "/fhir"}

	result, err := retrieveBGZ(t.Context(), source, "the-token", "999900006",
		[]string{nvi.CategoryPatient, nvi.CategoryCondition})

	require.NoError(t, err)
	require.Len(t, result.Queries, 1, "no clinical search may go out on a patient that was not settled")
	// Without these the test also passes when the first search never reached the
	// source: that too yields one query, no id, no items and an incomplete
	// result, and would mask the guard being removed.
	require.Equal(t, http.StatusOK, result.Queries[0].Status,
		"the first page was served; the continuation after it is what failed")
	require.Empty(t, result.Queries[0].Error)
	require.Contains(t, result.Queries[0].Truncated, "later page")
	require.Contains(t, result.Queries[0].Truncated, "more than one record",
		"and the chain has to say the identity was not settled, not only that a page was missed")
	require.Len(t, paths, 2, "page two was requested; it is its failure that stops the chain")
	require.Empty(t, result.PatientID)
	require.Empty(t, result.Items)
	require.True(t, result.Incomplete())
	for _, path := range paths {
		require.NotContains(t, path, "Condition", "and nothing may be asked about that patient")
	}
}
