package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// fhirID is the FHIR id datatype (https://hl7.org/fhir/R4/datatypes.html#id).
// An id is checked against it before it becomes part of a store URL.
var fhirID = regexp.MustCompile(`^[A-Za-z0-9\-.]{1,64}$`)

// validID reports whether an id from the path may become part of a store URL:
// a FHIR id, and not "." or "..", which the id pattern allows and a URL path
// would resolve.
func validID(id string) bool {
	return fhirID.MatchString(id) && id != "." && id != ".."
}

type server struct {
	store  store
	now    func() time.Time
	marker func() string
}

// newMux returns De Zonnebloem's EHR. now dates new records; marker proposes the
// marker word the form starts with.
func newMux(st store, now func() time.Time, marker func() string) *http.ServeMux {
	s := server{store: st, now: now, marker: marker}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /{$}", s.handleClients)
	mux.HandleFunc("GET /clients/{id}", s.handleClient)
	crossOrigin := http.NewCrossOriginProtection()
	mux.Handle("POST /clients/{id}/allergies", crossOrigin.Handler(http.HandlerFunc(s.handleAddAllergy)))
	mux.Handle("POST /clients/{id}/allergies/{allergyID}/remove", crossOrigin.Handler(http.HandlerFunc(s.handleRemoveAllergy)))
	return mux
}

func (s server) handleClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.demoClients(r.Context(), "")
	if err != nil {
		storeFailure(w, err)
		return
	}
	render(w, http.StatusOK, "clients.html", page{Title: "Clients", Clients: clients})
}

// demoClients lists the demo-pool clients as the screens show them today,
// marking the one whose record is open.
func (s server) demoClients(ctx context.Context, currentID string) ([]clientView, error) {
	clients, err := s.store.Clients(ctx)
	if err != nil {
		return nil, err
	}
	var views []clientView
	for _, c := range clients {
		if isPoolClientID(c.ID) {
			view := viewOf(c, s.now())
			view.Current = c.ID == currentID
			views = append(views, view)
		}
	}
	return views, nil
}

func (s server) handleClient(w http.ResponseWriter, r *http.Request) {
	c, ok := s.client(w, r)
	if !ok {
		return
	}
	form := allergyForm{
		Substance: substances[0].Code,
		Status:    statuses[0].Code,
		Note:      "Established after reaction. " + s.marker(),
	}
	s.renderClient(w, r, http.StatusOK, c, form, "")
}

func (s server) handleAddAllergy(w http.ResponseWriter, r *http.Request) {
	c, ok := s.client(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "unreadable form: "+err.Error(), http.StatusBadRequest)
		return
	}
	form := allergyFormFrom(r.PostForm)
	if reason := form.validate(); reason != "" {
		s.renderClient(w, r, http.StatusBadRequest, c, form, reason)
		return
	}
	if err := s.store.AddAllergy(r.Context(), allergyFor(c.ID, form, s.now())); err != nil {
		storeFailure(w, err)
		return
	}
	saved := "plain"
	if form.hasMarker() {
		saved = "marker"
	}
	http.Redirect(w, r, "/clients/"+url.PathEscape(c.ID)+"?saved="+saved, http.StatusSeeOther)
}

// handleRemoveAllergy removes an allergy added in the demo. Anything else, a
// seeded allergy or another client's, is not there to remove: seeded data stays
// so the NVI registration for its category keeps pointing at data that exists.
func (s server) handleRemoveAllergy(w http.ResponseWriter, r *http.Request) {
	c, ok := s.client(w, r)
	if !ok {
		return
	}
	id := r.PathValue("allergyID")
	if !validID(id) {
		http.NotFound(w, r)
		return
	}
	allergy, err := s.store.Allergy(r.Context(), id)
	if errors.Is(err, errNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		storeFailure(w, err)
		return
	}
	if stringOr(allergy.Patient.Reference) != "Patient/"+c.ID || !userCreated(allergy.Meta) {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteAllergy(r.Context(), id); err != nil {
		storeFailure(w, err)
		return
	}
	http.Redirect(w, r, "/clients/"+url.PathEscape(c.ID)+"?removed=1", http.StatusSeeOther)
}

// client resolves a demo-pool client: 404 outside the pool or when the store
// does not hold it, 502 when the store fails.
func (s server) client(w http.ResponseWriter, r *http.Request) (client, bool) {
	id := r.PathValue("id")
	if !validID(id) || !isPoolClientID(id) {
		http.NotFound(w, r)
		return client{}, false
	}
	c, err := s.store.Client(r.Context(), id)
	if errors.Is(err, errNotFound) {
		http.NotFound(w, r)
		return client{}, false
	}
	if err != nil {
		storeFailure(w, err)
		return client{}, false
	}
	return c, true
}

func (s server) renderClient(w http.ResponseWriter, r *http.Request, status int, c client, form allergyForm, formError string) {
	rec, err := s.store.Record(r.Context(), c.ID)
	if err != nil {
		storeFailure(w, err)
		return
	}
	clients, err := s.demoClients(r.Context(), c.ID)
	if err != nil {
		storeFailure(w, err)
		return
	}
	view := viewOf(c, s.now())
	render(w, status, "client.html", page{
		Title: c.Name, Clients: clients, Client: &view, Record: &rec, Form: form, FormError: formError,
		Substances: substances, Statuses: statuses,
		Saved: r.URL.Query().Get("saved"), Removed: r.URL.Query().Get("removed") != "",
	})
}

// storeFailure answers 502: the page depends on a store that did not answer
// properly, and an empty record would read as "this client has no data". The
// error names the request and its status; it carries no BSN and no note text.
func storeFailure(w http.ResponseWriter, err error) {
	slog.Error("zonnebloem-ehr: FHIR store request failed", "error", err)
	http.Error(w, "De Zonnebloem's record store did not answer properly: "+err.Error(), http.StatusBadGateway)
}

// isPoolClientID reports whether id is a demo-pool client: the only clients
// Plataan can retrieve and recycle, so the only ones these screens offer.
func isPoolClientID(id string) bool {
	for _, patient := range pool.Patients() {
		if patient.ZonnebloemPatientID == id {
			return true
		}
	}
	return false
}
