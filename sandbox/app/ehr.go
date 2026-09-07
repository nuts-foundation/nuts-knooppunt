package main

import (
	"net/http"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// patientRow is one row of the practitioner's patient list, and the header of
// the record and share screens.
type patientRow struct {
	Key        string
	Name       string
	BSN        string
	BirthDate  string
	Initials   string
	Shared     bool
	NVIUnknown bool
	Locked     bool // held by another session: "demo in progress"
	Mine       bool // held by this session
	Categories []string
}

// handlePatientList renders the practitioner's patients with their localization
// status.
//
// The NVI is queried once per patient, concurrently: the calls are independent
// and doing them in sequence would multiply one slow round trip by the size of
// the pool on the first screen after login. Each carries its own deadline, so an
// unreachable NVI costs one unknown chip rather than the page.
func (c Config) handlePatientList(w http.ResponseWriter, r *http.Request, session *authSession) {
	patients := pool.Patients()
	rows := make([]patientRow, len(patients))
	owner := lockOwner(session)

	var wg sync.WaitGroup
	for i, patient := range patients {
		mine := c.Locks.HeldBy(patient.Key, owner)
		rows[i] = patientRow{
			Key: patient.Key, Name: patientDisplayName(patient), BSN: patient.BSN,
			BirthDate: patient.BirthDate, Initials: patientInitials(patient),
			Locked: c.Locks.IsLocked(patient.Key) && !mine, Mine: mine,
		}
		wg.Add(1)
		go func(i int, bsn string) {
			defer wg.Done()
			status := c.patientShareStatus(r.Context(), bsn)
			rows[i].Shared, rows[i].NVIUnknown, rows[i].Categories = status.Shared, status.NVIUnknown, status.Categories
		}(i, patient.BSN)
	}
	wg.Wait()

	view := session.view()
	render(w, "ehr-patients.html", page{
		Title: "Patients · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Patients", ViewerOpen: true,
		Session: &view, Patients: rows, Notice: demoNotice(r),
	})
}

// patientDisplayName renders "Family, Given", the order a clinical list uses.
func patientDisplayName(p pool.PoolPatient) string {
	family := ""
	if p.Name.Family != nil {
		family = *p.Name.Family
	}
	given := ""
	if len(p.Name.Given) > 0 {
		given = p.Name.Given[0]
	}
	return family + ", " + given
}

func patientInitials(p pool.PoolPatient) string {
	initials := ""
	if len(p.Name.Given) > 0 {
		initials += firstLetter(p.Name.Given[0])
	}
	if p.Name.Family != nil {
		initials += firstLetter(*p.Name.Family)
	}
	return initials
}
