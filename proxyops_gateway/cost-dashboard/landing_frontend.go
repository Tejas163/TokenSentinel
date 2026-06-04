package main

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed landing.html
var landingContent embed.FS

var landingTmpl *template.Template

func init() {
	landingTmpl = template.Must(template.ParseFS(landingContent, "landing.html"))
}

func handleLanding(w http.ResponseWriter, r *http.Request) {
	landingTmpl.Execute(w, nil)
}
