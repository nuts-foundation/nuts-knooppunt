package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// patientRow is one row of the practitioner's patient list, and the header of
// the record and share screens.
type patientRow struct {
	Key                 string
	Name                string
	BSN                 string
	BirthDate           string
	Initials            string
	Shared              bool
	NVIUnknown          bool
	Subscribed          bool
	SubscriptionUnknown bool
	Locked              bool // held by another session: "demo in progress"
	Mine                bool // held by this session
	Categories          []string
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

// handleOpenPatient claims the patient for this session and redirects to the
// record.
//
// A POST, not a GET on the record itself. Acquiring the lock changes shared
// state: it denies another session access and releases this session's previous
// patient. RFC 9110 §9.2.1 makes safe methods essentially read-only from the
// client's perspective, and prefetchers, link scanners and retries rely on that.
// The patient row submits this on the same click, so the demo still costs one
// click.
func (c Config) handleOpenPatient(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site patient open is not allowed", http.StatusForbidden)
		return
	}
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	if !c.Locks.Switch(key, lockOwner(session)) {
		http.Redirect(w, r, "/demo/ehr?notice=patient-busy", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/demo/ehr/patients/"+key, http.StatusSeeOther)
}

// handlePatientRecord renders one patient's local record and its localization
// status. Read-only: the lock was taken by handleOpenPatient.
func (c Config) handlePatientRecord(w http.ResponseWriter, r *http.Request, session *authSession) {
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	row := c.patientRowFor(r.Context(), patient, session)
	view := session.view()
	render(w, "ehr-record.html", page{
		Title: row.Name + " · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Patient record", ViewerOpen: true,
		Session: &view, Patient: &row, Notice: demoNotice(r),
	})
}

// patientRowFor builds the header shared by the record and share screens.
func (c Config) patientRowFor(ctx context.Context, patient pool.PoolPatient, session *authSession) patientRow {
	status := c.patientShareStatus(ctx, patient.BSN)
	return patientRow{
		Key: patient.Key, Name: patientDisplayName(patient), BSN: patient.BSN,
		BirthDate: patient.BirthDate, Initials: patientInitials(patient),
		Shared: status.Shared, NVIUnknown: status.NVIUnknown, Categories: status.Categories,
		Subscribed: status.Subscribed, SubscriptionUnknown: status.SubscriptionUnknown,
		Mine: c.Locks.HeldBy(patient.Key, lockOwner(session)),
	}
}

// handleSubscribe retries only the Mitz step.
//
// This is what makes "no rollback" survivable. A patient whose localization
// records exist but whose consent subscription does not is a real state the flow
// can land in, and re-running the whole share to repair it would redo the NVI
// work for no reason.
func (c Config) handleSubscribe(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site subscribe is not allowed", http.StatusForbidden)
		return
	}
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	if !c.Locks.HeldBy(patient.Key, lockOwner(session)) {
		http.Error(w, "this session does not hold patient "+patient.Key+"; reopen the record", http.StatusConflict)
		return
	}
	if c.mitzSubscribe == nil {
		http.Redirect(w, r, "/demo/ehr/patients/"+patient.Key+"?notice=mitz-disabled", http.StatusSeeOther)
		return
	}
	if err := c.mitzSubscribe(r.Context(), patient.BSN); err != nil {
		http.Redirect(w, r, "/demo/ehr/patients/"+patient.Key+"?notice=mitz-retry-failed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/demo/ehr/patients/"+patient.Key+"?notice=mitz-retry-done", http.StatusSeeOther)
}
