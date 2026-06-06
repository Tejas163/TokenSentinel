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
	report, err := GetReport(appStore, id)
	if err != nil {
		log.Printf("get report error: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") || strings.HasPrefix(accept, "*/*") || accept == "" {
		reportJSON, _ := json.Marshal(report)
		reportTmpl.Execute(w, map[string]interface{}{
			"APIKey":     authAPIKey,
			"ReportJSON": template.JS(reportJSON),
		})
		return
	}

	encodeJSON(w, report)
}

func encodeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
