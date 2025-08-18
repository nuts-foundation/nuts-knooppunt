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

func RenderWithBase(w http.ResponseWriter, name string) {
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

	err = ts.ExecuteTemplate(w, "base", nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

}

// Route handling

func ServiceHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "service_list.html")
}

func ServiceNewHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "service_edit.html")
}

func ServiceNewPostHandler(w http.ResponseWriter, r *http.Request) {
	log.Print("Received new post request")

	r.ParseForm()
	var data = map[string]string{}
	data["resourceType"] = "HealthcareService"
	data["name"] = r.PostForm.Get("name")

	content, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := resourceCreate("HealthcareService", content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Print("New post created " + id)

	http.Redirect(w, r, "/mcsdadmin/healthcareservice", http.StatusFound)
}

func ServiceEditHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	RenderWithBase(w, "service_edit.html")
}

func HomePageHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Welcome to the homepage"))
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Page not found"))
}

func (c Component) RegisterHttpHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/mcsdadmin/healthcareservice", ServiceHandler)
	mux.HandleFunc("/mcsdadmin/healthcareservice/new", ServiceNewHandler)
	mux.HandleFunc("POST /mcsdadmin/healthcareservice/new", ServiceNewPostHandler)
	mux.HandleFunc("/mcsdadmin/healthcareservice/{id}/edit", ServiceEditHandler)
	mux.HandleFunc("PUT /mcsdadmin/healthcareservice/{id}/edit", ServiceEditHandler)
	mux.HandleFunc("/mcsdadmin", HomePageHandler)
	mux.HandleFunc("/mcsdadmin/", NotFoundHandler)
}
