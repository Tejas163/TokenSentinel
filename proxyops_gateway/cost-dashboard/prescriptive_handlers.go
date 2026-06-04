package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func handlePrescriptiveRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/prescriptive/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch parts[0] {
	case "assessments":
		if len(parts) == 1 {
			handleAssessments(w, r)
		} else if len(parts) == 2 {
			handleAssessments(w, r)
		} else if len(parts) == 3 && parts[2] == "versions" {
			handleAssessmentVersions(w, r)
		} else if len(parts) == 3 && parts[2] == "run" {
			handleRunAssessment(w, r)
		} else if len(parts) == 4 && parts[2] == "versions" {
			handleAssessmentVersions(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	case "report":
		if len(parts) >= 2 {
			id, err := strconv.Atoi(parts[1])
			if err != nil {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			if len(parts) == 2 {
				if r.Method == "GET" {
					handleReport(w, r, id)
				} else {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
			} else if len(parts) == 3 && parts[2] == "pdf" {
				handleReportDownload(w, r, id, "pdf")
			} else if len(parts) == 3 && parts[2] == "csv" {
				handleReportDownload(w, r, id, "csv")
			} else {
				http.Error(w, "not found", http.StatusNotFound)
			}
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	case "what-if":
		handleWhatIf(w, r)
	case "templates":
		handleTemplates(w, r)
	case "import":
		if len(parts) >= 2 && parts[1] == "csv" {
			handleImportCSV(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func handleAssessments(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/prescriptive/assessments")
	idStr = strings.Trim(idStr, "/")

	if idStr == "" {
		switch r.Method {
		case "GET":
			listAssessments(w, r)
		case "POST":
			createAssessment(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		getAssessment(w, r, id)
	case "PUT":
		updateAssessment(w, r, id)
	case "DELETE":
		deleteAssessment(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAssessmentVersions(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	assessmentID, err := strconv.Atoi(parts[3])
	if err != nil {
		http.Error(w, "invalid assessment id", http.StatusBadRequest)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if len(parts) >= 6 && parts[5] != "" {
		versionNum, err := strconv.Atoi(parts[5])
		if err != nil {
			http.Error(w, "invalid version number", http.StatusBadRequest)
			return
		}
		getAssessmentVersion(w, r, assessmentID, versionNum)
		return
	}

	listAssessmentVersions(w, r, assessmentID)
}

func listAssessments(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
		FROM assessments ORDER BY updated_at DESC`)
	if err != nil {
		log.Printf("list assessments error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var assessments []Assessment
	for rows.Next() {
		var a Assessment
		var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
			&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version, &createdAt, &updatedAt); err != nil {
			continue
		}
		json.Unmarshal(gpuJSON, &a.GPUConfigs)
		json.Unmarshal(tokenJSON, &a.TokenDistribution)
		json.Unmarshal(providerJSON, &a.ProvidersUsed)
		json.Unmarshal(teamJSON, &a.TeamComposition)
		a.CreatedAt = createdAt.Format(time.RFC3339)
		a.UpdatedAt = updatedAt.Format(time.RFC3339)
		assessments = append(assessments, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assessments)
}

func createAssessment(w http.ResponseWriter, r *http.Request) {
	var a Assessment
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if a.CompanyName == "" {
		http.Error(w, "company_name required", http.StatusBadRequest)
		return
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	if a.TokenDistribution.InputPct == 0 && a.TokenDistribution.OutputPct == 0 {
		a.TokenDistribution = TokenDistribution{InputPct: 0.7, OutputPct: 0.3}
	}

	gpuJSON, _ := json.Marshal(a.GPUConfigs)
	tokenJSON, _ := json.Marshal(a.TokenDistribution)
	providerJSON, _ := json.Marshal(a.ProvidersUsed)
	teamJSON, _ := json.Marshal(a.TeamComposition)

	var id int
	err := db.QueryRow(`INSERT INTO assessments (company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		a.CompanyName, a.CloudVendor, gpuJSON, a.MonthlyRequestVolume,
		tokenJSON, a.CurrentMonthlySpend, providerJSON, teamJSON, a.Source,
	).Scan(&id)
	if err != nil {
		log.Printf("create assessment error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	a.ID = id
	a.Version = 1
	a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	a.UpdatedAt = a.CreatedAt

	db.Exec(`INSERT INTO assessment_versions (assessment_id, version_number, snapshot) VALUES ($1, 1, $2)`,
		id, json.RawMessage(fmt.Sprintf(`{"id":%d}`, id)))

	log.Printf("audit: assessment created id=%d company=%s from %s", id, a.CompanyName, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(a)
}

func getAssessment(w http.ResponseWriter, r *http.Request, id int) {
	var a Assessment
	var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
	var createdAt, updatedAt time.Time

	err := db.QueryRow(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
		FROM assessments WHERE id = $1`, id).Scan(
		&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
		&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get assessment error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	json.Unmarshal(gpuJSON, &a.GPUConfigs)
	json.Unmarshal(tokenJSON, &a.TokenDistribution)
	json.Unmarshal(providerJSON, &a.ProvidersUsed)
	json.Unmarshal(teamJSON, &a.TeamComposition)
	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func updateAssessment(w http.ResponseWriter, r *http.Request, id int) {
	var existing Assessment
	err := db.QueryRow(`SELECT id, version FROM assessments WHERE id = $1`, id).Scan(&existing.ID, &existing.Version)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("update assessment lookup error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var a Assessment
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if a.CompanyName == "" {
		http.Error(w, "company_name required", http.StatusBadRequest)
		return
	}

	gpuJSON, _ := json.Marshal(a.GPUConfigs)
	tokenJSON, _ := json.Marshal(a.TokenDistribution)
	providerJSON, _ := json.Marshal(a.ProvidersUsed)
	teamJSON, _ := json.Marshal(a.TeamComposition)

	newVersion := existing.Version + 1
	_, err = db.Exec(`UPDATE assessments SET company_name=$1, cloud_vendor=$2, gpu_configs=$3,
		monthly_request_volume=$4, token_distribution=$5, current_monthly_spend=$6, providers_used=$7,
		team_composition=$8, source=$9, version=$10, updated_at=NOW() WHERE id=$11`,
		a.CompanyName, a.CloudVendor, gpuJSON, a.MonthlyRequestVolume,
		tokenJSON, a.CurrentMonthlySpend, providerJSON, teamJSON, a.Source, newVersion, id)
	if err != nil {
		log.Printf("update assessment error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	db.Exec(`INSERT INTO assessment_versions (assessment_id, version_number, snapshot) VALUES ($1, $2, $3)`,
		id, newVersion, json.RawMessage(fmt.Sprintf(`{"updated":true,"version":%d}`, newVersion)))

	a.ID = id
	a.Version = newVersion
	log.Printf("audit: assessment updated id=%d version=%d from %s", id, newVersion, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func deleteAssessment(w http.ResponseWriter, r *http.Request, id int) {
	result, err := db.Exec(`DELETE FROM assessments WHERE id = $1`, id)
	if err != nil {
		log.Printf("delete assessment error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	log.Printf("audit: assessment deleted id=%d from %s", id, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func listAssessmentVersions(w http.ResponseWriter, r *http.Request, assessmentID int) {
	rows, err := db.Query(`SELECT id, assessment_id, version_number, snapshot, created_at
		FROM assessment_versions WHERE assessment_id = $1 ORDER BY version_number DESC`, assessmentID)
	if err != nil {
		log.Printf("list versions error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var versions []AssessmentVersion
	for rows.Next() {
		var v AssessmentVersion
		var createdAt time.Time
		if err := rows.Scan(&v.ID, &v.AssessmentID, &v.VersionNumber, &v.Snapshot, &createdAt); err != nil {
			continue
		}
		v.CreatedAt = createdAt.Format(time.RFC3339)
		versions = append(versions, v)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

func getAssessmentVersion(w http.ResponseWriter, r *http.Request, assessmentID, versionNum int) {
	var v AssessmentVersion
	var createdAt time.Time
	err := db.QueryRow(`SELECT id, assessment_id, version_number, snapshot, created_at
		FROM assessment_versions WHERE assessment_id = $1 AND version_number = $2`,
		assessmentID, versionNum).Scan(&v.ID, &v.AssessmentID, &v.VersionNumber, &v.Snapshot, &createdAt)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get version error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	v.CreatedAt = createdAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func handleRunAssessment(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[3])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	report, err := RunAssessment(appStore, id)
	if err != nil {
		log.Printf("run assessment error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("audit: assessment run id=%d from %s", id, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}


func handleWhatIf(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[3])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var adjustments map[string]float64
	if err := json.NewDecoder(r.Body).Decode(&adjustments); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	projections, err := RunWhatIf(appStore, id, adjustments)
	if err != nil {
		log.Printf("what-if error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projections)
}

type StarterTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Assessment  Assessment `json:"assessment"`
}

func handleTemplates(w http.ResponseWriter, r *http.Request) {
	templates := []StarterTemplate{
		{
			Name:        "Startup",
			Description: "5 developers, OpenAI only, ~100K requests/mo",
			Assessment: Assessment{
				CompanyName:          "My Startup",
				CloudVendor:          "openai",
				MonthlyRequestVolume: 100000,
				TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
				CurrentMonthlySpend:  1500,
				ProvidersUsed:        []ProviderUsage{{Name: "openai", Models: []string{"gpt-4o-mini", "gpt-4o"}, MonthlySpend: 1500}},
				TeamComposition:      TeamComposition{Developers: 5, PlatformEngineers: 0, DevOps: 0, Management: 1},
				Source:               "manual",
			},
		},
		{
			Name:        "Mid-size",
			Description: "20 developers, multi-model (OpenAI + Anthropic), ~1M requests/mo",
			Assessment: Assessment{
				CompanyName:          "Mid-size Corp",
				CloudVendor:          "aws",
				MonthlyRequestVolume: 1000000,
				TokenDistribution:    TokenDistribution{InputPct: 0.75, OutputPct: 0.25},
				CurrentMonthlySpend:  12000,
				GPUConfigs:           []GPUConfig{{Type: "A100", Count: 4, Region: "us-east-1", HourlyPrice: 3.50, Reserved: true}},
				ProvidersUsed: []ProviderUsage{
					{Name: "openai", Models: []string{"gpt-4o", "gpt-4o-mini"}, MonthlySpend: 7000},
					{Name: "anthropic", Models: []string{"claude-3-sonnet"}, MonthlySpend: 5000},
				},
				TeamComposition: TeamComposition{Developers: 20, PlatformEngineers: 2, DevOps: 1, Management: 2},
				Source:          "manual",
			},
		},
		{
			Name:        "Enterprise",
			Description: "50+ developers, self-hosted infra, multi-provider, ~10M requests/mo",
			Assessment: Assessment{
				CompanyName:          "Enterprise Inc",
				CloudVendor:          "aws",
				MonthlyRequestVolume: 10000000,
				TokenDistribution:    TokenDistribution{InputPct: 0.8, OutputPct: 0.2},
				CurrentMonthlySpend:  85000,
				GPUConfigs: []GPUConfig{
					{Type: "H100", Count: 8, Region: "us-east-1", HourlyPrice: 4.50, Reserved: true},
					{Type: "A100", Count: 4, Region: "us-west-2", HourlyPrice: 3.50, Reserved: false},
				},
				ProvidersUsed: []ProviderUsage{
					{Name: "openai", Models: []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}, MonthlySpend: 35000},
					{Name: "anthropic", Models: []string{"claude-3-opus", "claude-3-sonnet"}, MonthlySpend: 25000},
					{Name: "self-hosted", Models: []string{"llama-3-70b", "llama-3-8b"}, MonthlySpend: 25000},
				},
				TeamComposition: TeamComposition{Developers: 50, PlatformEngineers: 5, DevOps: 3, Management: 4},
				Source:          "manual",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}
