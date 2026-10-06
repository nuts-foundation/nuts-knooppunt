package zonnebloem

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func validForm() allergyForm {
	return allergyForm{Substance: "762952008", Status: "confirmed", Note: "Na pinda. DEMO-BLUE-OTTER-12"}
}

func TestAllergyFormFrom_TrimsTheNote(t *testing.T) {
	form := allergyFormFrom(url.Values{"substance": {"762952008"}, "status": {"confirmed"}, "note": {"  DEMO-X  \n"}})

	require.Equal(t, allergyForm{Substance: "762952008", Status: "confirmed", Note: "DEMO-X"}, form)
}

func TestAllergyForm_Validate(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*allergyForm)
		reason string
	}{
		"valid":                 {func(*allergyForm) {}, ""},
		"no substance":          {func(f *allergyForm) { f.Substance = "" }, "Choose a substance from the list."},
		"substance not offered": {func(f *allergyForm) { f.Substance = "294505008" }, "Choose a substance from the list."},
		"status not offered":    {func(f *allergyForm) { f.Status = "refuted" }, "Choose a status from the list."},
		"empty note":            {func(f *allergyForm) { f.Note = "" }, ""},
		"500 multi-byte runes":  {func(f *allergyForm) { f.Note = strings.Repeat("é", 500) }, ""},
		"501 runes":             {func(f *allergyForm) { f.Note = strings.Repeat("é", 501) }, "The note is longer than 500 characters."},
	} {
		t.Run(name, func(t *testing.T) {
			form := validForm()
			tc.mutate(&form)
			require.Equal(t, tc.reason, form.validate())
		})
	}
}

func TestAllergyForm_HasMarker(t *testing.T) {
	require.True(t, validForm().hasMarker())
	require.False(t, allergyForm{Note: "Na pinda."}.hasMarker())
}

func TestAllergyFor_BuildsTheTaggedRecord(t *testing.T) {
	// 00:30 in Amsterdam is still the previous day in UTC; the date is UTC.
	now := time.Date(2026, 9, 29, 0, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	form := validForm()
	form.Status = "unconfirmed"

	a := allergyFor("pool-anna-zonnebloem-patient", form, now)

	require.Equal(t, []fhir.Coding{{System: strPtr(pool.UserCreatedTagSystem), Code: strPtr(pool.UserCreatedTagCode)}}, a.Meta.Tag)
	require.Equal(t, "Patient/pool-anna-zonnebloem-patient", *a.Patient.Reference)
	require.Equal(t, "http://snomed.info/sct", *a.Code.Coding[0].System)
	require.Equal(t, "762952008", *a.Code.Coding[0].Code)
	require.Equal(t, "Pinda", *a.Code.Coding[0].Display)
	require.Equal(t, "Pinda", *a.Code.Text)
	require.Equal(t, "active", *a.ClinicalStatus.Coding[0].Code)
	require.Equal(t, "http://terminology.hl7.org/CodeSystem/allergyintolerance-verification", *a.VerificationStatus.Coding[0].System)
	require.Equal(t, "unconfirmed", *a.VerificationStatus.Coding[0].Code)
	require.Equal(t, fhir.AllergyIntoleranceTypeAllergy, *a.Type)
	require.Equal(t, []fhir.AllergyIntoleranceCategory{fhir.AllergyIntoleranceCategoryFood}, a.Category)
	require.Equal(t, "2026-09-28", *a.RecordedDate)
	require.Equal(t, []fhir.Annotation{{Text: "Na pinda. DEMO-BLUE-OTTER-12"}}, a.Note)
}

func TestAllergyFor_LeavesOutAnEmptyNote(t *testing.T) {
	form := validForm()
	form.Note = ""

	require.Empty(t, allergyFor("p1", form, time.Now()).Note)
}

func TestAllergyFor_CategoryFollowsTheSubstance(t *testing.T) {
	form := validForm()
	form.Substance = "111088007" // Latex

	require.Equal(t, []fhir.AllergyIntoleranceCategory{fhir.AllergyIntoleranceCategoryEnvironment},
		allergyFor("p1", form, time.Now()).Category)
}

func TestSuggestedMarker_IsADemoMarker(t *testing.T) {
	first := func(int) int { return 0 }

	marker := suggestedMarker(first)

	require.Regexp(t, regexp.MustCompile(`^DEMO-[A-Z]+-[A-Z]+-\d{2}$`), marker)
	require.Equal(t, "DEMO-BLUE-BUTTERFLY-00", marker)
}

func strPtr(s string) *string { return &s }
