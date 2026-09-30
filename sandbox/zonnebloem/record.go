package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const bsnSystem = "http://fhir.nl/fhir/NamingSystem/bsn"

// client is one resident as the screens show them.
type client struct {
	ID   string
	Name string // given names first: "Anna Jansen"
	// sortKey orders the client list by family name, as a care home's list is.
	sortKey   string
	Initials  string
	BirthDate string
	BSN       string
}

// entry is one line of a client's record.
type entry struct {
	ID    string
	Title string
	// Meta is the line under the title: when an allergy was recorded, a dosage,
	// since when a condition applies.
	Meta string
	Note string
	// Status is an allergy's verification code; StatusLabel is what the screen
	// calls it.
	Status      string
	StatusLabel string
	// ClinicalStatus is an allergy's clinical status code: active, inactive or
	// resolved.
	ClinicalStatus string
	AddedInDemo    bool
}

// record is what the client screen shows below the banner.
type record struct {
	Allergies  []entry
	Medication []entry
	Conditions []entry
}

// AllergyAlert names the allergies that may still apply, for the banner. A
// refuted allergy, one entered in error and one that is inactive or resolved do
// not; one without a status might.
func (r record) AllergyAlert() string {
	var names []string
	for _, a := range r.Allergies {
		switch {
		case a.Status == "refuted" || a.Status == "entered-in-error" || a.lapsed():
		case a.Status == "unconfirmed":
			names = append(names, a.Title+" (suspected)")
		default:
			names = append(names, a.Title)
		}
	}
	return strings.Join(names, ", ")
}

// lapsed reports whether an allergy's clinical status says it no longer
// applies: inactive or resolved.
func (e entry) lapsed() bool {
	return e.ClinicalStatus == "inactive" || e.ClinicalStatus == "resolved"
}

func clientFrom(p fhir.Patient) client {
	c := client{ID: stringOr(p.Id), BirthDate: stringOr(p.BirthDate)}
	for _, identifier := range p.Identifier {
		if stringOr(identifier.System) == bsnSystem {
			c.BSN = stringOr(identifier.Value)
		}
	}
	if len(p.Name) > 0 {
		given := strings.Join(p.Name[0].Given, " ")
		family := stringOr(p.Name[0].Family)
		c.Name = joinNonEmpty(" ", given, family)
		c.sortKey = strings.ToLower(joinNonEmpty(", ", family, given))
		c.Initials = firstLetter(given) + firstLetter(family)
	}
	if c.Name == "" {
		c.Name = c.ID
		c.sortKey = strings.ToLower(c.ID)
	}
	return c
}

func allergyEntry(a fhir.AllergyIntolerance) entry {
	e := entry{
		ID:             stringOr(a.Id),
		Title:          titleOf(a.Code),
		Note:           noteText(a.Note),
		ClinicalStatus: codeOf(a.ClinicalStatus),
		AddedInDemo:    userCreated(a.Meta),
	}
	recorded := stringOr(a.RecordedDate)
	switch {
	case e.lapsed() && recorded != "":
		e.Meta = codeableText(a.ClinicalStatus) + ", recorded " + longDate(recorded)
	case e.lapsed():
		e.Meta = codeableText(a.ClinicalStatus)
	case recorded != "":
		e.Meta = "Recorded " + longDate(recorded)
	}
	e.Status, e.StatusLabel = verificationOf(a.VerificationStatus)
	return e
}

// verificationOf returns an allergy's verification code and what to call it:
// the form's own word for a status it offers, so the record and the form agree,
// and the store's display for any other.
func verificationOf(concept *fhir.CodeableConcept) (string, string) {
	code := codeOf(concept)
	if st, ok := statusByCode(code); ok {
		return code, st.Label
	}
	return code, codeableText(concept)
}

// codeOf returns the first code a concept carries.
func codeOf(concept *fhir.CodeableConcept) string {
	if concept == nil {
		return ""
	}
	for _, coding := range concept.Coding {
		if code := stringOr(coding.Code); code != "" {
			return code
		}
	}
	return ""
}

func medicationEntry(m fhir.MedicationRequest) entry {
	e := entry{Title: titleOf(m.MedicationCodeableConcept), AddedInDemo: userCreated(m.Meta)}
	if len(m.DosageInstruction) > 0 {
		e.Meta = stringOr(m.DosageInstruction[0].Text)
	}
	return e
}

func conditionEntry(c fhir.Condition) entry {
	e := entry{Title: titleOf(c.Code), AddedInDemo: userCreated(c.Meta)}
	if onset := stringOr(c.OnsetDateTime); onset != "" {
		e.Meta = "Since " + longDate(onset)
	}
	return e
}

// longDate writes a FHIR date or dateTime out in full, "12 March 1944", at the
// precision the store recorded. A value it cannot read is shown as it is.
func longDate(value string) string {
	if len(value) >= len(time.DateOnly) {
		if day, err := time.Parse(time.DateOnly, value[:len(time.DateOnly)]); err == nil {
			return day.Format("2 January 2006")
		}
	}
	if month, err := time.Parse("2006-01", value); err == nil {
		return month.Format("January 2006")
	}
	return value
}

// ageOn gives a client's age on a day, "82 years", or "" when the store holds no
// full birth date to count from.
func ageOn(birthDate string, now time.Time) string {
	born, err := time.Parse(time.DateOnly, birthDate)
	if err != nil {
		return ""
	}
	years := now.Year() - born.Year()
	if now.Month() < born.Month() || now.Month() == born.Month() && now.Day() < born.Day() {
		years--
	}
	if years == 1 {
		return "1 year"
	}
	return fmt.Sprintf("%d years", years)
}

// titleOf names an entry. A record without any code still gets a line, so
// nothing the store holds silently disappears from the screen.
func titleOf(concept *fhir.CodeableConcept) string {
	if text := codeableText(concept); text != "" {
		return text
	}
	return "(no description)"
}

// codeableText prefers the text a system wrote over a coding display, as
// Plataan's record does, and falls back to the bare code.
func codeableText(concept *fhir.CodeableConcept) string {
	if concept == nil {
		return ""
	}
	if text := stringOr(concept.Text); text != "" {
		return text
	}
	for _, coding := range concept.Coding {
		if display := stringOr(coding.Display); display != "" {
			return display
		}
	}
	for _, coding := range concept.Coding {
		if code := stringOr(coding.Code); code != "" {
			return code
		}
	}
	return ""
}

func userCreated(meta *fhir.Meta) bool {
	if meta == nil {
		return false
	}
	for _, tag := range meta.Tag {
		if stringOr(tag.System) == pool.UserCreatedTagSystem && stringOr(tag.Code) == pool.UserCreatedTagCode {
			return true
		}
	}
	return false
}

func noteText(notes []fhir.Annotation) string {
	var texts []string
	for _, note := range notes {
		if note.Text != "" {
			texts = append(texts, note.Text)
		}
	}
	return strings.Join(texts, " ")
}

func joinNonEmpty(separator string, parts ...string) string {
	var kept []string
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, separator)
}

func stringOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstLetter(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return ""
}
