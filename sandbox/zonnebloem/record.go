package main

import (
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const bsnSystem = "http://fhir.nl/fhir/NamingSystem/bsn"

// client is one resident as the screens show them.
type client struct {
	ID        string
	Name      string
	Initials  string
	BirthDate string
	BSN       string
}

// entry is one line of a client's record.
type entry struct {
	Title       string
	Detail      string
	AddedInDemo bool
}

// record is what the client screen shows below the banner.
type record struct {
	Allergies  []entry
	Medication []entry
	Conditions []entry
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
		switch {
		case family != "" && given != "":
			c.Name = family + ", " + given
		case family != "":
			c.Name = family
		default:
			c.Name = given
		}
		c.Initials = firstLetter(given) + firstLetter(family)
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	return c
}

func allergyEntry(a fhir.AllergyIntolerance) entry {
	return entry{
		Title:       titleOf(a.Code),
		Detail:      joinNonEmpty(" · ", codeableText(a.VerificationStatus), stringOr(a.RecordedDate), noteText(a.Note)),
		AddedInDemo: userCreated(a.Meta),
	}
}

func medicationEntry(m fhir.MedicationRequest) entry {
	e := entry{Title: titleOf(m.MedicationCodeableConcept), AddedInDemo: userCreated(m.Meta)}
	if len(m.DosageInstruction) > 0 {
		e.Detail = stringOr(m.DosageInstruction[0].Text)
	}
	return e
}

func conditionEntry(c fhir.Condition) entry {
	return entry{Title: titleOf(c.Code), Detail: stringOr(c.OnsetDateTime), AddedInDemo: userCreated(c.Meta)}
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
