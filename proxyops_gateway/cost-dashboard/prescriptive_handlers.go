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

	"github.com/proxyops/internal/engine"
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
	case "marketplace":
		handleMarketplace(w, r)
	case "routing-rules":
		handleRoutingRules(w, r)
	case "variance":
		if len(parts) >= 2 {
			id, err := strconv.Atoi(parts[1])
			if err != nil {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			handleVariance(w, r, id)
		} else {
			http.Error(w, "assessment id required", http.StatusBadRequest)
		}
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

type AssessmentVersion struct {
	ID            int             `json:"id"`
	AssessmentID  int             `json:"assessment_id"`
	VersionNumber int             `json:"version_number"`
	Snapshot      json.RawMessage `json:"snapshot"`
	CreatedAt     string          `json:"created_at"`
}

func listAssessments(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 100)
	offset := parseIntParam(r, "offset", 0)
	orgID := getOrgID(r)

	var rows *sql.Rows
	var err error
	if orgID != "" {
		rows, err = db.Query(`SELECT id, org_id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
			token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
			FROM assessments WHERE org_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	} else {
		rows, err = db.Query(`SELECT id, org_id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
			token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
			FROM assessments ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		log.Printf("list assessments error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var assessments []engine.Assessment
	for rows.Next() {
		var a engine.Assessment
		var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&a.ID, &a.OrgID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
			&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version, &createdAt, &updatedAt); err != nil {
			log.Printf("scan assessment: %v", err)
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
	var a engine.Assessment
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if a.CompanyName == "" {
		http.Error(w, "company_name required", http.StatusBadRequest)
		return
	}
	if a.MonthlyRequestVolume < 0 {
		http.Error(w, "monthly_request_volume must be non-negative", http.StatusBadRequest)
		return
	}
	if a.CurrentMonthlySpend < 0 {
		http.Error(w, "current_monthly_spend must be non-negative", http.StatusBadRequest)
		return
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	if a.TokenDistribution.InputPct == 0 && a.TokenDistribution.OutputPct == 0 {
		a.TokenDistribution = engine.TokenDistribution{InputPct: 0.7, OutputPct: 0.3}
	}
	a.OrgID = getOrgID(r)

	gpuJSON, _ := json.Marshal(a.GPUConfigs)
	tokenJSON, _ := json.Marshal(a.TokenDistribution)
	providerJSON, _ := json.Marshal(a.ProvidersUsed)
	teamJSON, _ := json.Marshal(a.TeamComposition)

	var id int
	err := db.QueryRow(`INSERT INTO assessments (org_id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		a.OrgID, a.CompanyName, a.CloudVendor, gpuJSON, a.MonthlyRequestVolume,
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
	var a engine.Assessment
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
	var existing struct {
		ID      int
		Version int
	}
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

	var a engine.Assessment
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if a.CompanyName == "" {
		http.Error(w, "company_name required", http.StatusBadRequest)
		return
	}
	if a.MonthlyRequestVolume < 0 {
		http.Error(w, "monthly_request_volume must be non-negative", http.StatusBadRequest)
		return
	}
	if a.CurrentMonthlySpend < 0 {
		http.Error(w, "current_monthly_spend must be non-negative", http.StatusBadRequest)
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
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)
	rows, err := db.Query(`SELECT id, assessment_id, version_number, snapshot, created_at
		FROM assessment_versions WHERE assessment_id = $1 ORDER BY version_number DESC LIMIT $2 OFFSET $3`, assessmentID, limit, offset)
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
			log.Printf("scan assessment version: %v", err)
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
	report, err := engine.RunAssessment(appStore, id)
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
	projections, err := engine.RunWhatIf(appStore, id, adjustments)
	if err != nil {
		log.Printf("what-if error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projections)
}

type StarterTemplate struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Assessment  engine.Assessment `json:"assessment"`
}

func handleTemplates(w http.ResponseWriter, r *http.Request) {
	templates := []StarterTemplate{
		{
			Name:        "Startup",
			Description: "5 developers, OpenAI only, ~100K requests/mo",
			Assessment: engine.Assessment{
				CompanyName:          "My Startup",
				CloudVendor:          "openai",
				MonthlyRequestVolume: 100000,
				TokenDistribution:    engine.TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
				CurrentMonthlySpend:  1500,
				ProvidersUsed:        []engine.ProviderUsage{{Name: "openai", Models: []string{"gpt-4o-mini", "gpt-4o"}, MonthlySpend: 1500}},
				TeamComposition:      engine.TeamComposition{Developers: 5, PlatformEngineers: 0, DevOps: 0, Management: 1},
				Source:               "manual",
			},
		},
		{
			Name:        "Mid-size",
			Description: "20 developers, multi-model (OpenAI + Anthropic), ~1M requests/mo",
			Assessment: engine.Assessment{
				CompanyName:          "Mid-size Corp",
				CloudVendor:          "aws",
				MonthlyRequestVolume: 1000000,
				TokenDistribution:    engine.TokenDistribution{InputPct: 0.75, OutputPct: 0.25},
				CurrentMonthlySpend:  12000,
				GPUConfigs:           []engine.GPUConfig{{Type: "A100", Count: 4, Region: "us-east-1", HourlyPrice: 3.50, Reserved: true}},
				ProvidersUsed: []engine.ProviderUsage{
					{Name: "openai", Models: []string{"gpt-4o", "gpt-4o-mini"}, MonthlySpend: 7000},
					{Name: "anthropic", Models: []string{"claude-3-sonnet"}, MonthlySpend: 5000},
				},
				TeamComposition: engine.TeamComposition{Developers: 20, PlatformEngineers: 2, DevOps: 1, Management: 2},
				Source:          "manual",
			},
		},
		{
			Name:        "Enterprise",
			Description: "50+ developers, self-hosted infra, multi-provider, ~10M requests/mo",
			Assessment: engine.Assessment{
				CompanyName:          "Enterprise Inc",
				CloudVendor:          "aws",
				MonthlyRequestVolume: 10000000,
				TokenDistribution:    engine.TokenDistribution{InputPct: 0.8, OutputPct: 0.2},
				CurrentMonthlySpend:  85000,
				GPUConfigs: []engine.GPUConfig{
					{Type: "H100", Count: 8, Region: "us-east-1", HourlyPrice: 4.50, Reserved: true},
					{Type: "A100", Count: 4, Region: "us-west-2", HourlyPrice: 3.50, Reserved: false},
				},
				ProvidersUsed: []engine.ProviderUsage{
					{Name: "openai", Models: []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}, MonthlySpend: 35000},
					{Name: "anthropic", Models: []string{"claude-3-opus", "claude-3-sonnet"}, MonthlySpend: 25000},
					{Name: "self-hosted", Models: []string{"llama-3-70b", "llama-3-8b"}, MonthlySpend: 25000},
				},
				TeamComposition: engine.TeamComposition{Developers: 50, PlatformEngineers: 5, DevOps: 3, Management: 4},
				Source:          "manual",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}

type MarketplaceTemplate struct {
	ID            int               `json:"id"`
	OrgID         string            `json:"-"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Category      string            `json:"category"`
	TemplateData  engine.Assessment  `json:"template_data"`
	Tags          []string          `json:"tags"`
	DownloadCount int               `json:"download_count"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
}

func handleMarketplace(w http.ResponseWriter, r *http.Request) {
	orgID := getOrgID(r)
	switch r.Method {
	case "GET":
		listMarketplaceTemplates(w, r, orgID)
	case "POST":
		createMarketplaceTemplate(w, r, orgID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listMarketplaceTemplates(w http.ResponseWriter, r *http.Request, orgID string) {
	category := r.URL.Query().Get("category")
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	var rows *sql.Rows
	var err error
	query := `SELECT id, name, description, category, template_data, tags, download_count, created_at, updated_at
		FROM marketplace_templates`
	var args []interface{}
	argIdx := 1

	var conditions []string
	if orgID != "" {
		conditions = append(conditions, fmt.Sprintf("org_id = $%d", argIdx))
		args = append(args, orgID)
		argIdx++
	}
	if category != "" {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, category)
		argIdx++
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY download_count DESC, created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err = db.Query(query, args...)
	if err != nil {
		log.Printf("list marketplace error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []MarketplaceTemplate
	for rows.Next() {
		var t MarketplaceTemplate
		var dataJSON []byte
		var tags []string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Category, &dataJSON, &tags, &t.DownloadCount, &createdAt, &updatedAt); err != nil {
			log.Printf("scan marketplace template: %v", err)
			continue
		}
		json.Unmarshal(dataJSON, &t.TemplateData)
		t.Tags = tags
		t.CreatedAt = createdAt.Format(time.RFC3339)
		t.UpdatedAt = updatedAt.Format(time.RFC3339)
		templates = append(templates, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}

func createMarketplaceTemplate(w http.ResponseWriter, r *http.Request, orgID string) {
	var t MarketplaceTemplate
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if t.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if t.Category == "" {
		t.Category = "general"
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}

	dataJSON, _ := json.Marshal(t.TemplateData)
	tagsArray := "{}"
	if len(t.Tags) > 0 {
		tagList := ""
		for i, tag := range t.Tags {
			if i > 0 {
				tagList += ","
			}
			tagList += fmt.Sprintf("%q", tag)
		}
		tagsArray = "{" + tagList + "}"
	}

	var id int
	var createdAt, updatedAt time.Time
	err := db.QueryRow(
		`INSERT INTO marketplace_templates (org_id, name, description, category, template_data, tags)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at, updated_at`,
		orgID, t.Name, t.Description, t.Category, dataJSON, tagsArray,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		log.Printf("create marketplace template error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	t.ID = id
	t.CreatedAt = createdAt.Format(time.RFC3339)
	t.UpdatedAt = updatedAt.Format(time.RFC3339)

	log.Printf("audit: marketplace template created id=%d name=%s from %s", id, t.Name, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
}

// Handle per-template operations: GET /api/prescriptive/marketplace/{id}, PUT, DELETE
// Note: these are routed via the path already consumed by handleMarketplace being called from handlePrescriptiveRouter.
// The router matches "marketplace" and calls handleMarketplace. For sub-routes like /marketplace/{id},
// we need to parse the path here. This is a known limitation of the flat router.
// For now, full CRUD is available via POST (create) and GET (list). Individual operations
// can be added by extending the router path matching.

func handleRoutingRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(req.Models) == 0 {
		http.Error(w, "models array required", http.StatusBadRequest)
		return
	}
	rules := engine.GetRoutingRules(req.Models)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func handleVariance(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ActualCosts map[string]float64 `json:"actual_costs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(req.ActualCosts) == 0 {
		http.Error(w, "actual_costs map required", http.StatusBadRequest)
		return
	}
	entries, err := engine.CompareProjections(appStore, id, req.ActualCosts)
	if err != nil {
		log.Printf("variance error: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
