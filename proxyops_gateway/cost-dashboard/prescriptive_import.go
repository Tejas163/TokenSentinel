package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type CSVColumnMapping struct {
	ModelColumn       int `json:"model_column"`
	InputTokensColumn int `json:"input_tokens_column"`
	OutputTokensColumn int `json:"output_tokens_column"`
	CostColumn        int `json:"cost_column"`
	TimestampColumn   int `json:"timestamp_column"`
	TeamColumn        int `json:"team_column"`
	HasHeader         bool `json:"has_header"`
	AssessmentID      int  `json:"assessment_id"`
}

func handleImportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		importCSVWithMapping(w, r)
	} else {
		var mapping CSVColumnMapping
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&mapping); err != nil {
			http.Error(w, "invalid body: send JSON with column mapping or multipart file", http.StatusBadRequest)
			return
		}
		if mapping.AssessmentID <= 0 {
			http.Error(w, "assessment_id required and must be positive", http.StatusBadRequest)
			return
		}

		rows, err := readCSVFromBody(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read CSV: %v", err), http.StatusBadRequest)
			return
		}

		count := processCSVRows(mapping.AssessmentID, rows, mapping)
		slog.Info("csv import", "assessment", mapping.AssessmentID, "rows", count, "from", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"assessment_id": mapping.AssessmentID,
			"rows_imported": count,
		})
	}
}

func importCSVWithMapping(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field 'file' required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	allRows, err := reader.ReadAll()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read CSV: %v", err), http.StatusBadRequest)
		return
	}
	if len(allRows) == 0 {
		http.Error(w, "empty CSV", http.StatusBadRequest)
		return
	}

	assessmentIDStr := r.FormValue("assessment_id")
	assessmentID, err := strconv.Atoi(assessmentIDStr)
	if err != nil || assessmentID <= 0 {
		http.Error(w, "assessment_id required and must be positive", http.StatusBadRequest)
		return
	}

	hasHeader := r.FormValue("has_header") == "true"

	mapping := CSVColumnMapping{
		ModelColumn:        columnIndex(r.FormValue("model_column")),
		InputTokensColumn:  columnIndex(r.FormValue("input_tokens_column")),
		OutputTokensColumn: columnIndex(r.FormValue("output_tokens_column")),
		CostColumn:         columnIndex(r.FormValue("cost_column")),
		TimestampColumn:    columnIndex(r.FormValue("timestamp_column")),
		TeamColumn:         columnIndex(r.FormValue("team_column")),
		HasHeader:          hasHeader,
		AssessmentID:       assessmentID,
	}

	startIdx := 0
	if hasHeader {
		startIdx = 1
	}

	count := 0
	parsedCount := 0
	for i := startIdx; i < len(allRows); i++ {
		row := allRows[i]
		if len(row) <= mapping.ModelColumn {
			continue
		}
		model := strings.TrimSpace(row[mapping.ModelColumn])
		if model == "" {
			continue
		}
		parsedCount++

		var inputTokens, outputTokens int64
		if mapping.InputTokensColumn >= 0 && mapping.InputTokensColumn < len(row) {
			inputTokens, _ = strconv.ParseInt(strings.TrimSpace(row[mapping.InputTokensColumn]), 10, 64)
		}
		if mapping.OutputTokensColumn >= 0 && mapping.OutputTokensColumn < len(row) {
			outputTokens, _ = strconv.ParseInt(strings.TrimSpace(row[mapping.OutputTokensColumn]), 10, 64)
		}

		provider := "imported"
		cost := 0.0
		if mapping.CostColumn >= 0 && mapping.CostColumn < len(row) {
			cost, _ = strconv.ParseFloat(strings.TrimSpace(row[mapping.CostColumn]), 64)
		}

		inputM := float64(inputTokens) / 1_000_000
		outputM := float64(outputTokens) / 1_000_000

		if assessmentID > 0 {
			db.Exec(`INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'imported')`,
				assessmentID, model, provider, cost, cost, inputM, outputM)
		}
		count++
	}

	slog.Info("csv import", "assessment", assessmentID, "parsed", parsedCount, "imported", count, "from", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"assessment_id": assessmentID,
		"rows_parsed":   parsedCount,
		"rows_imported": count,
	})
}

func readCSVFromBody(r *http.Request) ([][]string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(body)))
	return reader.ReadAll()
}

func processCSVRows(assessmentID int, rows [][]string, mapping CSVColumnMapping) int {
	startIdx := 0
	if mapping.HasHeader {
		startIdx = 1
	}

	count := 0
	for i := startIdx; i < len(rows); i++ {
		row := rows[i]
		if len(row) <= mapping.ModelColumn {
			continue
		}
		model := strings.TrimSpace(row[mapping.ModelColumn])
		if model == "" {
			continue
		}

		var inputTokens, outputTokens int64
		if mapping.InputTokensColumn >= 0 && mapping.InputTokensColumn < len(row) {
			inputTokens, _ = strconv.ParseInt(strings.TrimSpace(row[mapping.InputTokensColumn]), 10, 64)
		}
		if mapping.OutputTokensColumn >= 0 && mapping.OutputTokensColumn < len(row) {
			outputTokens, _ = strconv.ParseInt(strings.TrimSpace(row[mapping.OutputTokensColumn]), 10, 64)
		}

		provider := "imported"
		cost := 0.0
		if mapping.CostColumn >= 0 && mapping.CostColumn < len(row) {
			cost, _ = strconv.ParseFloat(strings.TrimSpace(row[mapping.CostColumn]), 64)
		}
		inputM := float64(inputTokens) / 1_000_000
		outputM := float64(outputTokens) / 1_000_000

		if assessmentID > 0 {
			db.Exec(`INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'imported')`,
				assessmentID, model, provider, cost, cost, inputM, outputM)
		}
		count++
	}
	return count
}

func columnIndex(s string) int {
	if s == "" {
		return -1
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return i
}
