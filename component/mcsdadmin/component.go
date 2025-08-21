package mcsdadmin

import (
	"context"
	"html/template"
	"net/http"

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

func RenderWithBase(w http.ResponseWriter, name string, data any) {
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

func ServiceHandler(w http.ResponseWriter, r *http.Request) {
	services, err := FindAllServices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "service_list.html", services)
}

func ServiceNewHandler(w http.ResponseWriter, r *http.Request) {
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
	RenderWithBase(w, "service_edit.html", props)
}

func ServiceNewPostHandler(w http.ResponseWriter, r *http.Request) {
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

	RenderWithBase(w, "service_list.html", services)
}

func OrganizationHandler(w http.ResponseWriter, r *http.Request) {
	organizations, err := FindAllOrganizations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "organization_list.html", organizations)
}

func OrganizationNewHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "organization_edit.html", nil)
}

func OrganizationNewPostHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for organization resource")

	resourceType := "Organization"

	err := r.ParseForm()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse form input")
		return
	}

	var data = map[string]string{}
	data["resourceType"] = resourceType
	data["name"] = r.PostForm.Get("name")
	data["active"] = r.PostForm.Get("active")

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

	RenderWithBase(w, "organization_list.html", organizations)
}

func HomePageHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "home.html", nil)
}

func NotImplementedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	RenderWithBase(w, "home.html", nil)
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Path not implemented"))
}

func (c Component) RegisterHttpHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/mcsdadmin/healthcareservice", ServiceHandler)
	mux.HandleFunc("/mcsdadmin/healthcareservice/new", ServiceNewHandler)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/new", ServiceNewPostHandler)
	mux.HandleFunc("/mcsdadmin/healthcareservice/{id}/edit", NotImplementedHandler)
	mux.HandleFunc("PUT /mcsdadmin/healthcareservice/{id}/edit", NotImplementedHandler)
	mux.HandleFunc("/mcsdadmin/organization", OrganizationHandler)
	mux.HandleFunc("/mcsdadmin/organization/new", OrganizationNewHandler)
	mux.HandleFunc("POST /mcsdadmin/organization/new", OrganizationNewPostHandler)
	mux.HandleFunc("/mcsdadmin", HomePageHandler)
	mux.HandleFunc("/mcsdadmin/", NotFoundHandler)
}
