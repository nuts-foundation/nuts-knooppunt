package zonnebloem

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const (
	snomedSystem       = "http://snomed.info/sct"
	clinicalSystem     = "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical"
	verificationSystem = "http://terminology.hl7.org/CodeSystem/allergyintolerance-verification"

	// maxNoteLength caps the free-text note, in characters. Anyone who can reach a
	// hosted instance can type into it, and what they type shows up highlighted
	// in every demo that retrieves the client.
	maxNoteLength = 500

	// markerPrefix is the convention Plataan highlights (sandbox/app retrieval.go).
	markerPrefix = "DEMO-"
)

// substance is one choice in the form. Each code is a SNOMED CT substance
// concept, confirmed as "(substance)" on tx.fhir.org; the labels are Dutch, as
// the seeded clinical data is.
type substance struct {
	Code     string
	Label    string
	Category fhir.AllergyIntoleranceCategory
}

var substances = []substance{
	{Code: "762952008", Label: "Pinda", Category: fhir.AllergyIntoleranceCategoryFood},
	{Code: "102263004", Label: "Ei", Category: fhir.AllergyIntoleranceCategoryFood},
	{Code: "735029006", Label: "Schaal- en schelpdieren", Category: fhir.AllergyIntoleranceCategoryFood},
	{Code: "387207008", Label: "Ibuprofen", Category: fhir.AllergyIntoleranceCategoryMedication},
	{Code: "111088007", Label: "Latex", Category: fhir.AllergyIntoleranceCategoryEnvironment},
	{Code: "288328004", Label: "Bijengif", Category: fhir.AllergyIntoleranceCategoryEnvironment},
}

// status is one verification status the form offers: Code and Display from the
// allergyintolerance-verification code system, Label for the form.
type status struct {
	Code    string
	Display string
	Label   string
}

var statuses = []status{
	{Code: "confirmed", Display: "Confirmed", Label: "Confirmed"},
	{Code: "unconfirmed", Display: "Unconfirmed", Label: "Suspected"},
}

// allergyForm is a submitted form, kept as entered so a rejected submission
// renders again with the user's own values.
type allergyForm struct {
	Substance string
	Status    string
	Note      string
}

func allergyFormFrom(values url.Values) allergyForm {
	return allergyForm{
		Substance: values.Get("substance"),
		Status:    values.Get("status"),
		Note:      strings.TrimSpace(values.Get("note")),
	}
}

// validate returns why the form cannot be saved, or "" when it can.
func (f allergyForm) validate() string {
	if _, ok := substanceByCode(f.Substance); !ok {
		return "Choose a substance from the list."
	}
	if _, ok := statusByCode(f.Status); !ok {
		return "Choose a status from the list."
	}
	if utf8.RuneCountInString(f.Note) > maxNoteLength {
		return fmt.Sprintf("The note is longer than %d characters.", maxNoteLength)
	}
	return ""
}

// hasMarker reports whether the note carries the word Plataan highlights.
func (f allergyForm) hasMarker() bool {
	return strings.Contains(f.Note, markerPrefix)
}

// allergyFor builds the record for a validated form. The id is left to the
// store, and the tag is what recycle deletes by.
func allergyFor(clientID string, f allergyForm, now time.Time) fhir.AllergyIntolerance {
	sub, _ := substanceByCode(f.Substance)
	st, _ := statusByCode(f.Status)
	allergy := fhir.AllergyIntolerance{
		Meta: &fhir.Meta{Tag: []fhir.Coding{{
			System: to.Ptr(pool.UserCreatedTagSystem), Code: to.Ptr(pool.UserCreatedTagCode),
		}}},
		ClinicalStatus: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(clinicalSystem), Code: to.Ptr("active"), Display: to.Ptr("Active"),
		}}},
		VerificationStatus: &fhir.CodeableConcept{Coding: []fhir.Coding{{
			System: to.Ptr(verificationSystem), Code: to.Ptr(st.Code), Display: to.Ptr(st.Display),
		}}},
		Type:     to.Ptr(fhir.AllergyIntoleranceTypeAllergy),
		Category: []fhir.AllergyIntoleranceCategory{sub.Category},
		Code: &fhir.CodeableConcept{
			Coding: []fhir.Coding{{System: to.Ptr(snomedSystem), Code: to.Ptr(sub.Code), Display: to.Ptr(sub.Label)}},
			Text:   to.Ptr(sub.Label),
		},
		Patient:      fhir.Reference{Reference: to.Ptr("Patient/" + clientID)},
		RecordedDate: to.Ptr(now.UTC().Format(time.DateOnly)),
	}
	if f.Note != "" {
		allergy.Note = []fhir.Annotation{{Text: f.Note}}
	}
	return allergy
}

func substanceByCode(code string) (substance, bool) {
	for _, s := range substances {
		if s.Code == code {
			return s, true
		}
	}
	return substance{}, false
}

func statusByCode(code string) (status, bool) {
	for _, s := range statuses {
		if s.Code == code {
			return s, true
		}
	}
	return status{}, false
}

var markerWords = [2][]string{
	{"BLUE", "GREEN", "AMBER", "SILVER", "CORAL", "IVORY", "OLIVE", "RUBY"},
	{"BUTTERFLY", "HERON", "OTTER", "MAPLE", "LANTERN", "PEBBLE", "WILLOW", "COMET"},
}

// suggestedMarker proposes a marker word for the note, so each demo shows a
// different one. intN is math/rand/v2's IntN in production.
func suggestedMarker(intN func(int) int) string {
	return fmt.Sprintf("%s%s-%s-%02d", markerPrefix,
		markerWords[0][intN(len(markerWords[0]))], markerWords[1][intN(len(markerWords[1]))], intN(100))
}
