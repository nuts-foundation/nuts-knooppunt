package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// storeAgainst runs a fhirStore against handler, standing in for De Zonnebloem's
// HAPI tenant. Handlers run on the server's goroutine, so they check with assert:
// require's FailNow may only be called from the test's own goroutine.
func storeAgainst(t *testing.T, handler http.HandlerFunc) *fhirStore {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	require.NoError(t, err)
	return newFHIRStore(base, server.Client())
}

func writeFHIR(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	assert.NoError(t, json.NewEncoder(w).Encode(body))
}

func searchSet(resources ...any) map[string]any {
	entries := make([]map[string]any, 0, len(resources))
	for _, r := range resources {
		entries = append(entries, map[string]any{"resource": r})
	}
	return map[string]any{"resourceType": "Bundle", "type": "searchset", "entry": entries}
}

func notFound() map[string]any {
	return map[string]any{"resourceType": "OperationOutcome", "issue": []any{
		map[string]any{"severity": "error", "code": "not-found", "diagnostics": "HAPI-2001: Resource Patient/x is not known"},
	}}
}

// The screens show given names first, but the list goes by family name, as a
// care home's does: Dirk Bakker comes before Anna Visser.
func TestFHIRStore_ClientsSortsByFamilyNameAndSkipsAnOperationOutcome(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/Patient/_search", r.URL.Path)
		writeFHIR(t, w, http.StatusOK, searchSet(
			fhir.Patient{Id: to.Ptr("p2"), Name: []fhir.HumanName{{Given: []string{"Anna"}, Family: to.Ptr("Visser")}}},
			map[string]any{"resourceType": "OperationOutcome", "id": "warning"},
			fhir.Patient{Id: to.Ptr("p1"), Name: []fhir.HumanName{{Given: []string{"Dirk"}, Family: to.Ptr("Bakker")}}},
		))
	})

	clients, err := st.Clients(t.Context())

	require.NoError(t, err)
	require.Len(t, clients, 2, "the OperationOutcome is not a client")
	require.Equal(t, "Dirk Bakker", clients[0].Name)
	require.Equal(t, "Anna Visser", clients[1].Name)
}

func TestFHIRStore_ClientReportsAnUnknownClientAsNotFound(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		writeFHIR(t, w, http.StatusNotFound, notFound())
	})

	_, err := st.Client(t.Context(), "x")

	require.ErrorIs(t, err, errNotFound)
}

// A store that fails is not a store without the client: reporting it as 404
// would tell the presenter the resident does not exist.
func TestFHIRStore_ClientReportsAFailingStoreAsAnError(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := st.Client(t.Context(), "x")

	require.Error(t, err)
	require.NotErrorIs(t, err, errNotFound)
}

func TestFHIRStore_RecordSearchesThisClientAndListsDemoAdditionsFirst(t *testing.T) {
	params := map[string]url.Values{}
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		params[r.URL.Path] = r.PostForm
		switch r.URL.Path {
		case "/AllergyIntolerance/_search":
			writeFHIR(t, w, http.StatusOK, searchSet(
				fhir.AllergyIntolerance{Id: to.Ptr("seeded"), Code: &fhir.CodeableConcept{Text: to.Ptr("Penicilline")}},
				fhir.AllergyIntolerance{Id: to.Ptr("added"), Meta: userCreatedMeta(), Code: &fhir.CodeableConcept{Text: to.Ptr("Pinda")}},
			))
		case "/MedicationRequest/_search":
			writeFHIR(t, w, http.StatusOK, searchSet(fhir.MedicationRequest{
				MedicationCodeableConcept: &fhir.CodeableConcept{Text: to.Ptr("Metoprolol")},
			}))
		case "/Condition/_search":
			writeFHIR(t, w, http.StatusOK, searchSet(fhir.Condition{Code: &fhir.CodeableConcept{Text: to.Ptr("Hypertensie")}}))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	rec, err := st.Record(t.Context(), "pool-anna-zonnebloem-patient")

	require.NoError(t, err)
	require.Equal(t, "Patient/pool-anna-zonnebloem-patient", params["/AllergyIntolerance/_search"].Get("patient"))
	require.Equal(t, "Patient/pool-anna-zonnebloem-patient", params["/MedicationRequest/_search"].Get("subject"))
	require.Equal(t, "Patient/pool-anna-zonnebloem-patient", params["/Condition/_search"].Get("subject"))
	require.Equal(t, []string{"Pinda", "Penicilline"}, []string{rec.Allergies[0].Title, rec.Allergies[1].Title})
	require.Equal(t, "Metoprolol", rec.Medication[0].Title)
	require.Equal(t, "Hypertensie", rec.Conditions[0].Title)
}

// HAPI answers an identical search from its cache for a while (60 seconds by
// default), so the record page right after a save showed the client without the
// allergy just added. Every search asks for a fresh result.
func TestFHIRStore_SearchesAskForFreshResults(t *testing.T) {
	cacheControl := map[string]string{}
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		cacheControl[r.URL.Path] = r.Header.Get("Cache-Control")
		writeFHIR(t, w, http.StatusOK, searchSet())
	})

	_, err := st.Clients(t.Context())
	require.NoError(t, err)
	_, err = st.Record(t.Context(), "p1")
	require.NoError(t, err)

	require.Equal(t, map[string]string{
		"/Patient/_search":            "no-cache",
		"/AllergyIntolerance/_search": "no-cache",
		"/MedicationRequest/_search":  "no-cache",
		"/Condition/_search":          "no-cache",
	}, cacheControl)
}

func TestFHIRStore_AddAllergyPostsTheResource(t *testing.T) {
	var posted fhir.AllergyIntolerance
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/AllergyIntolerance", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.NoError(t, json.Unmarshal(body, &posted))
		posted.Id = to.Ptr("new-id")
		writeFHIR(t, w, http.StatusCreated, posted)
	})

	err := st.AddAllergy(t.Context(), fhir.AllergyIntolerance{
		Meta:    userCreatedMeta(),
		Patient: fhir.Reference{Reference: to.Ptr("Patient/p1")},
	})

	require.NoError(t, err)
	require.Equal(t, "Patient/p1", *posted.Patient.Reference)
	require.True(t, userCreated(posted.Meta), "the tag must reach the store")
}

func TestFHIRStore_AddAllergyReportsARejection(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	})

	require.Error(t, st.AddAllergy(t.Context(), fhir.AllergyIntolerance{}))
}

func TestFHIRStore_AllergyReadsOneByID(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/AllergyIntolerance/a1", r.URL.Path)
		writeFHIR(t, w, http.StatusOK, fhir.AllergyIntolerance{
			Id: to.Ptr("a1"), Meta: userCreatedMeta(), Patient: fhir.Reference{Reference: to.Ptr("Patient/p1")},
		})
	})

	allergy, err := st.Allergy(t.Context(), "a1")

	require.NoError(t, err)
	require.Equal(t, "Patient/p1", stringOr(allergy.Patient.Reference))
	require.True(t, userCreated(allergy.Meta))
}

// HAPI answers 404 for an allergy it never held and 410 for one already removed:
// a second removal of the same allergy finds nothing, not a failing store.
func TestFHIRStore_AllergyReportsAnUnknownOrRemovedAllergyAsNotFound(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
			writeFHIR(t, w, status, notFound())
		})

		_, err := st.Allergy(t.Context(), "a1")

		require.ErrorIsf(t, err, errNotFound, "status %d", status)
	}
}

func TestFHIRStore_AllergyReportsAFailingStoreAsAnError(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := st.Allergy(t.Context(), "a1")

	require.Error(t, err)
	require.NotErrorIs(t, err, errNotFound)
}

func TestFHIRStore_DeleteAllergyDeletesThatAllergy(t *testing.T) {
	var method, path string
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		writeFHIR(t, w, http.StatusOK, map[string]any{"resourceType": "OperationOutcome", "issue": []any{
			map[string]any{"severity": "information", "code": "informational", "diagnostics": "Successfully deleted 1 resource(s)."},
		}})
	})

	require.NoError(t, st.DeleteAllergy(t.Context(), "a1"))
	require.Equal(t, http.MethodDelete, method)
	require.Equal(t, "/AllergyIntolerance/a1", path)
}

func TestFHIRStore_DeleteAllergyReportsARejection(t *testing.T) {
	st := storeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	require.Error(t, st.DeleteAllergy(t.Context(), "a1"))
}
