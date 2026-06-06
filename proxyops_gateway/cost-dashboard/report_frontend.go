package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

func handleReportFrontend(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/prescriptive/report/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			key := r.Header.Get("X-Api-Key")
			if key == "" {
				key = r.URL.Query().Get("api_key")
			}
			if key == "" || key != authAPIKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			report, err := GetReport(appStore, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			encodeJSON(w, report)
			return
		}
		report, err := GetReport(appStore, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		reportJSON, _ := json.Marshal(report)
		reportTmpl.Execute(w, map[string]interface{}{
			"APIKey":     authAPIKey,
			"ReportJSON": template.JS(reportJSON),
		})
		return
	}

	if len(parts) >= 2 && (parts[1] == "csv" || parts[1] == "pdf") {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			if b := r.Header.Get("Authorization"); len(b) > 7 && strings.EqualFold(b[:7], "Bearer ") {
				key = b[7:]
			}
		}
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if key == "" || key != authAPIKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handleReportDownload(w, r, id, parts[1])
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}
