package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
)

//go:embed report.html
var reportContent embed.FS

var reportTmpl *template.Template

func init() {
	reportTmpl = template.Must(template.ParseFS(reportContent, "report.html"))
}

func handleReport(w http.ResponseWriter, r *http.Request, id int) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") || strings.HasPrefix(accept, "*/*") || accept == "" {
		reportTmpl.Execute(w, map[string]string{"APIKey": authAPIKey})
		return
	}

	report, err := GetReport(id)
	if err != nil {
		log.Printf("get report error: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, report)
}

func encodeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
