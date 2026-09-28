package main

import (
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func userCreatedMeta() *fhir.Meta {
	return &fhir.Meta{Tag: []fhir.Coding{{System: to.Ptr(pool.UserCreatedTagSystem), Code: to.Ptr(pool.UserCreatedTagCode)}}}
}

func TestClientFrom_NameInitialsBirthDateAndBSN(t *testing.T) {
	got := clientFrom(fhir.Patient{
		Id:         to.Ptr("pool-anna-zonnebloem-patient"),
		Identifier: []fhir.Identifier{{System: to.Ptr(bsnSystem), Value: to.Ptr("999900006")}},
		Name:       []fhir.HumanName{{Given: []string{"Anna"}, Family: to.Ptr("Jansen")}},
		BirthDate:  to.Ptr("1944-03-12"),
	})

	require.Equal(t, client{
		ID: "pool-anna-zonnebloem-patient", Name: "Jansen, Anna", Initials: "AJ",
		BirthDate: "1944-03-12", BSN: "999900006",
	}, got)
}

func TestClientFrom_WithoutANameFallsBackToTheID(t *testing.T) {
	require.Equal(t, "p1", clientFrom(fhir.Patient{Id: to.Ptr("p1")}).Name)
}

func TestAllergyEntry_StatusDateNoteAndTheDemoTag(t *testing.T) {
	got := allergyEntry(fhir.AllergyIntolerance{
		Meta: userCreatedMeta(),
		Code: &fhir.CodeableConcept{Text: to.Ptr("Pinda")},
		VerificationStatus: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			Code: to.Ptr("confirmed"), Display: to.Ptr("Confirmed"),
		}}},
		RecordedDate: to.Ptr("2026-09-28"),
		Note:         []fhir.Annotation{{Text: "Na pinda. DEMO-BLUE-OTTER-12"}},
	})

	require.Equal(t, entry{
		Title: "Pinda", Detail: "Confirmed · 2026-09-28 · Na pinda. DEMO-BLUE-OTTER-12", AddedInDemo: true,
	}, got)
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

	require.Equal(t, entry{Title: "Metformine", Detail: "Metformine 500 mg 2dd"}, medication)
	require.Equal(t, entry{Title: "Hypertensie", Detail: "2009"}, condition)
}
