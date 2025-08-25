package templates

import (
	"embed"
	"html/template"
	"log"
	"net/http"
)

//go:embed *.html
var tmplFS embed.FS

func RenderWithBase(w http.ResponseWriter, name string, data any) {
	files := []string{
		"base.html",
		name,
	}

	ts, err := template.ParseFS(tmplFS, files...)
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
