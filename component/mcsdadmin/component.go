package mcsdadmin

import (
	"context"
	"encoding/json"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"html/template"
	"net/http"
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

// Helpers

func ResourceFromMap(resourceType string, data map[string]string) (string, error) {
	content, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	id, err := CreateResource(resourceType, content)
	if err != nil {
		return "", err
	}

	log.Debug().Msg("New resource created " + id)
	return id, nil
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
	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "service_edit.html", nil)
}

func ServiceNewPostHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug().Msg("New post for HealthcareService resource")

	resourceType := "HealthcareService"

	r.ParseForm()
	var data = map[string]string{}
	data["resourceType"] = resourceType
	data["name"] = r.PostForm.Get("name")

	_, err := ResourceFromMap(resourceType, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

	services, err := FindAllServices()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find all services")
	}

	RenderWithBase(w, "service_list.html", services)
}

func ServiceEditHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	RenderWithBase(w, "service_edit.html", nil)
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

	r.ParseForm()
	var data = map[string]string{}
	data["resourceType"] = "Organization"
	data["name"] = r.PostForm.Get("name")
	data["active"] = r.PostForm.Get("active")

	content, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := CreateResource("Organization", content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msg("New resource created " + id)
	w.WriteHeader(http.StatusCreated)
	RenderWithBase(w, "organization_list.html", nil)
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
