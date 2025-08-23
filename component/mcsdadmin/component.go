package mcsdadmin

import (
	"context"
	"html/template"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
}

func New() *Component {
	return &Component{}
}

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	// Nothing to do
	return nil
}

// Template rendering

const templateFolder = "./component/mcsdadmin/templates/"

func renderWithBase(w http.ResponseWriter, name string, data any) {
	files := []string{
		templateFolder + "base.html",
		templateFolder + name,
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

}

// Route handling

func listServices(w http.ResponseWriter, r *http.Request) {
	services, err := FindAllServices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "service_list.html", services)
}

func newService(w http.ResponseWriter, r *http.Request) {
	organizations, err := FindAllOrganizations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		Organizations []fhir.Organization
	}{
		Organizations: organizations,
	}

	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "service_edit.html", props)
}

func newServicePost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for HealthcareService resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var service fhir.HealthcareService
	name := r.PostForm.Get("name")
	service.Name = &name
	active := r.PostForm.Get("active") == "true"
	service.Active = &active

	reference := "Organization/" + r.PostForm.Get("providedById")
	refType := "Organization"
	service.ProvidedBy = &fhir.Reference{
		Reference: &reference,
		Type:      &refType,
	}

	var providedByOrg fhir.Organization
	err = client.Read(reference, &providedByOrg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find referred organisation")
		return
	}
	service.ProvidedBy.Display = providedByOrg.Name

	_, err = CreateHealthcareService(service)

	w.WriteHeader(http.StatusCreated)

	services, err := FindAllServices()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all services")
	}

	renderWithBase(w, "service_list.html", services)
}

func listOrganizations(w http.ResponseWriter, r *http.Request) {
	organizations, err := FindAllOrganizations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "organization_list.html", organizations)
}

func newOrganization(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "organization_edit.html", nil)
}

func newOrganizationPost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for organization resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var org fhir.Organization
	name := r.PostForm.Get("name")
	org.Name = &name
	active := r.PostForm.Get("active") == "true"
	org.Active = &active

	_, err = CreateOrganisation(org)

	w.WriteHeader(http.StatusCreated)

	organizations, err := FindAllOrganizations()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all organizations")
	}

	renderWithBase(w, "organization_list.html", organizations)
}

func listEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := FindAllEndpoints()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "endpoint_list.html", endpoints)
}

func newEndpoint(w http.ResponseWriter, r *http.Request) {
	organizations, err := FindAllOrganizations()
	status, err := valuesets.CodingsFrom("endpoint-status")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := struct {
		Organizations []fhir.Organization
		Status        []fhir.Coding
	}{
		Organizations: organizations,
		Status:        status,
	}

	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "endpoint_edit.html", props)
}

func newEndpointPost(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for Endpoint resource")

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var endpoint fhir.Endpoint
	address := r.PostForm.Get("address")
	endpoint.Address = address

	reference := "Organization/" + r.PostForm.Get("managingOrg")
	refType := "Organization"
	endpoint.ManagingOrganization = &fhir.Reference{
		Reference: &reference,
		Type:      &refType,
	}

	var managingOrg fhir.Organization
	err = client.Read(reference, &managingOrg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find referred organisation")
		return
	}
	endpoint.ManagingOrganization.Display = managingOrg.Name

	// TODO: Doesn't make sense to me that this status field is an enum not a string
	//status := r.PostForm.Get("status")
	//endpoint.Status = &status

	_, err = CreateEndpoint(endpoint)

	w.WriteHeader(http.StatusCreated)

	endpoints, err := FindAllEndpoints()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all endpoints")
	}

	renderWithBase(w, "endpoint_list.html", endpoints)
}

func homePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	renderWithBase(w, "home.html", nil)
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	renderWithBase(w, "home.html", nil)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Path not implemented"))
}

func (c Component) RegisterHttpHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /mcsdadmin/healthcareservice", listServices)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/new", newService)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/new", newServicePost)
	mux.HandleFunc("GET /mcsdadmin/healthcareservice/{id}/edit", notImplemented)
	mux.HandleFunc("PUT /mcsdadmin/healthcareservice/{id}/edit", notImplemented)
	mux.HandleFunc("GET /mcsdadmin/organization", listOrganizations)
	mux.HandleFunc("GET /mcsdadmin/organization/new", newOrganization)
	mux.HandleFunc("POST /mcsdadmin/organization/new", newOrganizationPost)
	mux.HandleFunc("GET /mcsdadmin/endpoint", listEndpoints)
	mux.HandleFunc("GET /mcsdadmin/endpoint/new", newEndpoint)
	mux.HandleFunc("POST /mcsdadmin/endpoint/new", newEndpointPost)
	mux.HandleFunc("GET /mcsdadmin", homePage)
	mux.HandleFunc("GET /mcsdadmin/", notFound)
}
