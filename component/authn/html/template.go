package html

import (
	"embed"
	"html/template"
)

var loginTemplate *template.Template

//go:embed *.html
var fs embed.FS

func init() {
	loginTemplate = template.Must(template.ParseFS(fs, "login.html"))
}
