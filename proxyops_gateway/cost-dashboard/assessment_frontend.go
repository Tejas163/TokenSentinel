package main

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed assessments.html
var assessmentContent embed.FS

var assessmentTmpl *template.Template

func init() {
	assessmentTmpl = template.Must(template.ParseFS(assessmentContent, "assessments.html"))
}

func handleAssessmentFrontend(w http.ResponseWriter, r *http.Request) {
	assessmentTmpl.Execute(w, map[string]string{"APIKey": authAPIKey})
}
