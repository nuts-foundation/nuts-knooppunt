package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// sectionOrder is the order the record screen lists its sections in, following
// the wireframe rather than whatever order the categories arrive in.
var sectionOrder = []string{"Allergies", "Medication", "Conditions"}

// localizedRecord is what the NVI answered about one organization: it holds data
// about this patient, in these data categories.
type localizedRecord struct {
	CustodianURA string
	Categories   []string
}

// localizedSource is one row of the retrieve screen. It keeps the index's answer
// and the directory's apart, because they come from two different generic
// functions and the screen exists to show which of them established what: the
// NVI names a holder and never an address, GF Addressing turns that name into
// one.
type localizedSource struct {
	URA        string
	Categories []string
	Name       string
	Address    string
	AddressErr string
}

// Addressable reports whether the directory produced an address to retrieve
// from. A holder without one is still a true finding of the index.
func (s localizedSource) Addressable() bool { return s.Address != "" }

// DisplayName falls back to the URA when the directory could not name the
// holder, so an unaddressable source is still identified by what the index did
// establish rather than rendering as a blank card.
func (s localizedSource) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return "Organization " + s.URA
}

func (s localizedSource) Initial() string { return firstLetter(s.DisplayName()) }

// External reports whether this line came from another organization, which is
// what the record screen colours the source chip by.
func (i recordItem) External() bool { return i.Source != plataanName() }

// recordSection groups retrieved and local items for display.
type recordSection struct {
	Name  string
	Items []recordItem
}

// retrievalStore keeps what each session retrieved, per patient. In memory and
// process-local, like the lock registry: a demo run is one sitting in one
// process and nothing here is meant to outlive it. Keyed by lock owner so that
// ending a session discards its retrievals along with its locks, and so that one
// practitioner's run never renders on another's screen.
type retrievalStore struct {
	mu      sync.Mutex
	byOwner map[string]map[string]sourceRetrieval
}

func newRetrievalStore() *retrievalStore {
	return &retrievalStore{byOwner: map[string]map[string]sourceRetrieval{}}
}

func (s *retrievalStore) put(owner, patientKey string, retrieval sourceRetrieval) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byOwner[owner] == nil {
		s.byOwner[owner] = map[string]sourceRetrieval{}
	}
	s.byOwner[owner][patientKey] = retrieval
}

func (s *retrievalStore) get(owner, patientKey string) (sourceRetrieval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	retrieval, ok := s.byOwner[owner][patientKey]
	return retrieval, ok
}

func (s *retrievalStore) clearOwner(owner string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byOwner, owner)
}

// localizeSources runs the two lookup legs of the chain: the NVI says which
// organizations hold data about this patient, the directory says where each can
// be reached. A holder the directory cannot address is kept, with the reason, so
// the screen can report a half-finished chain instead of an absent source.
func (c Config) localizeSources(ctx context.Context, bsn string) ([]localizedSource, error) {
	if c.nviLocalize == nil {
		return nil, errors.New("the sandbox is not wired to the Knooppunt in this environment")
	}
	records, err := c.nviLocalize(ctx, bsn)
	if err != nil {
		return nil, err
	}

	var sources []localizedSource
	for _, record := range records {
		// Our own registration describes the record already on screen. Offering
		// it as a source would present De Plataan's own data as an external find.
		if record.CustodianURA == plataanURA {
			continue
		}
		source := localizedSource{URA: record.CustodianURA, Categories: record.Categories}
		switch {
		case c.mcsdResolve == nil:
			source.AddressErr = "the directory is not configured in this environment"
		default:
			resolved, err := c.mcsdResolve(ctx, record.CustodianURA)
			if err != nil {
				source.AddressErr = err.Error()
			} else {
				source.Name, source.Address = resolved.Name, resolved.Address
			}
		}
		sources = append(sources, source)
	}
	return sources, nil
}

// handleRetrieveForm renders where the patient's data can be found. Read-only
// and deliberately so: this is the screen the practitioner confirms from, and
// #532's acceptance criterion is that nothing is retrieved before they do.
func (c Config) handleRetrieveForm(w http.ResponseWriter, r *http.Request, session *authSession) {
	patient, ok := pool.PatientByKey(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown patient: "+r.PathValue("key"), http.StatusNotFound)
		return
	}
	row := c.rowFor(patient, c.patientShareStatus(r.Context(), patient.BSN), session)

	sources, err := c.localizeSources(r.Context(), patient.BSN)
	rendered := page{
		Title: "Retrieve · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Retrieve data", ViewerOpen: true,
		Patient: &row, Sources: sources,
	}
	if err != nil {
		rendered.LocalizeErr = err.Error()
	}
	view := session.view()
	rendered.Session = &view
	render(w, "ehr-retrieve.html", rendered)
}

// handleRetrieve runs the confirmed chain against one source and renders the
// authorization result.
func (c Config) handleRetrieve(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site retrieval is not allowed", http.StatusForbidden)
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "unreadable form", http.StatusBadRequest)
		return
	}

	// The chain is re-run rather than carried over from the form: the page the
	// practitioner confirmed from is a claim by the browser, and a URA it names
	// has to be one the index really reports before anything is asked of it.
	sources, err := c.localizeSources(r.Context(), patient.BSN)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var chosen localizedSource
	for _, source := range sources {
		if source.URA == r.FormValue("ura") && source.Addressable() {
			chosen = source
			break
		}
	}
	if chosen.URA == "" {
		http.Error(w, "no addressable source with URA "+r.FormValue("ura")+" holds data for this patient",
			http.StatusConflict)
		return
	}
	if c.retrieveFromSource == nil {
		http.Error(w, "retrieval is not wired up in this environment", http.StatusBadGateway)
		return
	}

	retrieval, err := c.retrieveFromSource(r.Context(), *session,
		sourceAddress{URA: chosen.URA, Name: chosen.Name, Address: chosen.Address},
		patient.BSN, chosen.Categories)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	c.Retrievals.put(lockOwner(session), patient.Key, retrieval)

	row := c.rowFor(patient, c.patientShareStatus(r.Context(), patient.BSN), session)
	view := session.view()
	render(w, "ehr-authorization.html", page{
		Title: "Authorization · Plataan EHR", Guise: "ehr",
		Scenario: scenario, ShowReset: true,
		BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
		Active: "dossier", TopTitle: "Authorization · " + chosen.Name, ViewerOpen: true,
		Session: &view, Patient: &row, Retrieval: &retrieval,
	})
}

// recordSections merges De Plataan's own resources with whatever this session
// retrieved, every line carrying the organization it came from. A denied
// retrieval contributes nothing, which is what keeps the record from claiming a
// second source it was refused.
func (c Config) recordSections(patient pool.PoolPatient, session *authSession) ([]recordSection, []string) {
	items := localItems(patient)
	sourceNames := []string{plataanName()}

	if retrieval, ok := c.Retrievals.get(lockOwner(session), patient.Key); ok && len(retrieval.Items) > 0 {
		items = append(items, retrieval.Items...)
		sourceNames = append(sourceNames, retrieval.Source.Name)
	}

	byName := map[string][]recordItem{}
	for _, item := range items {
		byName[item.Section] = append(byName[item.Section], item)
	}
	var sections []recordSection
	for _, name := range sectionOrder {
		if grouped := byName[name]; len(grouped) > 0 {
			sections = append(sections, recordSection{Name: name, Items: grouped})
		}
	}
	return sections, sourceNames
}

// localItems renders De Plataan's own resources as record lines, so that "every
// element carries its source" holds for the hospital's own half too. It reuses
// the retrieval's own item builder rather than a parallel one, so the two halves
// of the record cannot be rendered by different rules.
func localItems(patient pool.PoolPatient) []recordItem {
	var items []recordItem
	for _, resource := range patient.PlataanResources() {
		var category string
		switch resource.(type) {
		case *fhir.Condition:
			category = nvi.CategoryCondition
		case *fhir.MedicationRequest:
			category = nvi.CategoryMedicationRequest
		case *fhir.AllergyIntolerance:
			category = nvi.CategoryAllergyIntolerance
		default:
			continue
		}
		raw, err := json.Marshal(resource)
		if err != nil {
			continue
		}
		item, ok := itemFrom(raw, bgzSearches[category])
		if !ok {
			continue
		}
		item.Source = plataanName()
		items = append(items, item)
	}
	return items
}

// plataanName takes the hospital's name from the seed rather than restating it,
// so the record cannot attribute data to a name the directory does not use.
func plataanName() string {
	if name := plataan.Organization().Name; name != nil {
		return *name
	}
	return "Ziekenhuis De Plataan"
}
