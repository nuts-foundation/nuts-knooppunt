package main

import (
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func userCreatedMeta() *fhir.Meta {
	return &fhir.Meta{Tag: []fhir.Coding{{System: to.Ptr(pool.UserCreatedTagSystem), Code: to.Ptr(pool.UserCreatedTagCode)}}}
}

func verification(code, display string) *fhir.CodeableConcept {
	return &fhir.CodeableConcept{Coding: []fhir.Coding{{
		System: to.Ptr(verificationSystem), Code: to.Ptr(code), Display: to.Ptr(display),
	}}}
}

func TestClientFrom_NameInitialsBirthDateAndBSN(t *testing.T) {
	got := clientFrom(fhir.Patient{
		Id:         to.Ptr("pool-anna-zonnebloem-patient"),
		Identifier: []fhir.Identifier{{System: to.Ptr(bsnSystem), Value: to.Ptr("999900006")}},
		Name:       []fhir.HumanName{{Given: []string{"Anna"}, Family: to.Ptr("Jansen")}},
		BirthDate:  to.Ptr("1944-03-12"),
	})

	require.Equal(t, client{
		ID: "pool-anna-zonnebloem-patient", Name: "Anna Jansen", sortKey: "jansen, anna", Initials: "AJ",
		BirthDate: "1944-03-12", BSN: "999900006",
	}, got)
}

func TestClientFrom_WithoutANameFallsBackToTheID(t *testing.T) {
	require.Equal(t, "p1", clientFrom(fhir.Patient{Id: to.Ptr("p1")}).Name)
}

func TestLongDate(t *testing.T) {
	for in, want := range map[string]string{
		"1944-03-12":                "12 March 1944",
		"2026-09-28":                "28 September 2026",
		"2012-03-04T10:00:00+01:00": "4 March 2012",
		"2019-05":                   "May 2019",
		"2019":                      "2019",
		"":                          "",
		"not a date":                "not a date",
	} {
		require.Equalf(t, want, longDate(in), "longDate(%q)", in)
	}
}

func TestAgeOn(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for born, want := range map[string]string{
		"1944-03-12": "82 years",
		"1948-09-30": "77 years", // the birthday is tomorrow
		"1948-09-29": "78 years", // the birthday is today
		"2025-09-01": "1 year",
		"1944":       "", // without a full date there is no age to give
		"":           "",
	} {
		require.Equalf(t, want, ageOn(born, now), "ageOn(%q)", born)
	}
}

func TestAllergyEntry_IDStatusDateNoteAndTheDemoTag(t *testing.T) {
	got := allergyEntry(fhir.AllergyIntolerance{
		Id:                 to.Ptr("a1"),
		Meta:               userCreatedMeta(),
		Code:               &fhir.CodeableConcept{Text: to.Ptr("Pinda")},
		VerificationStatus: verification("confirmed", "Confirmed"),
		RecordedDate:       to.Ptr("2026-09-28"),
		Note:               []fhir.Annotation{{Text: "Na pinda. DEMO-BLUE-OTTER-12"}},
	})

	require.Equal(t, entry{
		ID: "a1", Title: "Pinda", Meta: "Recorded 28 September 2026", Note: "Na pinda. DEMO-BLUE-OTTER-12",
		Status: "confirmed", StatusLabel: "Confirmed", AddedInDemo: true,
	}, got)
}

// The form offers "Suspected" for an unconfirmed allergy; the record uses the
// same word for it, whatever display the store holds.
func TestAllergyEntry_UnconfirmedReadsAsSuspected(t *testing.T) {
	got := allergyEntry(fhir.AllergyIntolerance{VerificationStatus: verification("unconfirmed", "Unconfirmed")})

	require.Equal(t, "Suspected", got.StatusLabel)
}

func TestAllergyEntry_AStatusTheFormDoesNotOfferKeepsItsDisplay(t *testing.T) {
	got := allergyEntry(fhir.AllergyIntolerance{VerificationStatus: verification("refuted", "Refuted")})

	require.Equal(t, "refuted", got.Status)
	require.Equal(t, "Refuted", got.StatusLabel)
}

func TestAllergyEntry_WithoutACodeStillHasATitle(t *testing.T) {
	require.Equal(t, "(no description)", allergyEntry(fhir.AllergyIntolerance{}).Title)
}

func TestUserCreated_IgnoresOtherTags(t *testing.T) {
	other := &fhir.Meta{Tag: []fhir.Coding{{System: to.Ptr(pool.UserCreatedTagSystem), Code: to.Ptr("seeded")}}}
	require.False(t, allergyEntry(fhir.AllergyIntolerance{Meta: other}).AddedInDemo)
}

func TestMedicationAndConditionEntries(t *testing.T) {
	medication := medicationEntry(fhir.MedicationRequest{
		MedicationCodeableConcept: &fhir.CodeableConcept{Text: to.Ptr("Metformine")},
		DosageInstruction:         []fhir.Dosage{{Text: to.Ptr("Metformine 500 mg 2dd")}},
	})
	condition := conditionEntry(fhir.Condition{
		Code: &fhir.CodeableConcept{Coding: []fhir.Coding{{Display: to.Ptr("Hypertensie")}}}, OnsetDateTime: to.Ptr("2009"),
	})

	require.Equal(t, entry{Title: "Metformine", Meta: "Metformine 500 mg 2dd"}, medication)
	require.Equal(t, entry{Title: "Hypertensie", Meta: "Since 2009"}, condition)
}

// The banner's alert names every allergy that may still apply: a refuted one or
// one entered in error does not, and one without a status might.
func TestRecord_AllergyAlert(t *testing.T) {
	rec := record{Allergies: []entry{
		{Title: "Pinda", Status: "confirmed"},
		{Title: "Penicilline", Status: "unconfirmed"},
		{Title: "Latex", Status: "refuted"},
		{Title: "Ei", Status: "entered-in-error"},
		{Title: "Bijengif"},
	}}

	require.Equal(t, "Pinda, Penicilline (suspected), Bijengif", rec.AllergyAlert())
	require.Empty(t, record{}.AllergyAlert())
}
