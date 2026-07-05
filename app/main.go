package main

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"tokensentinel/engine"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

//go:embed landing.html report.html sample-billing.csv
var templateFS embed.FS

var (
	store  engine.Store
	nextID atomic.Int64
)

func init() {
	nextID.Store(1)
	dbPath := os.Getenv("TS_DB_PATH")
	if dbPath == "" {
		dbPath = "tokensentinel.db"
	}
	s, err := engine.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	store = s
}

type ReportPage struct {
	Report *engine.AssessmentReport
	ID     int
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleLanding)
	mux.HandleFunc("/analyze", handleAnalyze)
	mux.HandleFunc("/import", handleImport)
	mux.HandleFunc("/report/", handleReport)
	mux.HandleFunc("/sample-billing.csv", handleSampleCSV)

	log.Printf("TokenSentinel app listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

var funcMap = template.FuncMap{
	"replace": func(old, new, src string) string {
		return strings.ReplaceAll(src, old, new)
	},
	"mul": func(a, b float64) float64 {
		return a * b
	},
	"div": func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return a / b
	},
}

func parseTmpl(files ...string) *template.Template {
	return template.Must(template.New(files[0]).Funcs(funcMap).ParseFS(templateFS, files...))
}

func handleSampleCSV(w http.ResponseWriter, r *http.Request) {
	data, err := templateFS.ReadFile("sample-billing.csv")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=sample-billing.csv")
	w.Write(data)
}

func handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	parseTmpl("landing.html").Execute(w, nil)
}

type columnMap struct {
	model       int
	provider    int
	inputTokens int
	outputTokens int
	cost        int
}

var knownHeaders = map[string]string{
	"model":             "model",
	"model_name":        "model",
	"model_id":          "model",
	"llm":               "model",
	"input_tokens":      "input_tokens",
	"prompt_tokens":     "input_tokens",
	"input":             "input_tokens",
	"output_tokens":     "output_tokens",
	"completion_tokens": "output_tokens",
	"output":            "output_tokens",
	"cost":              "cost",
	"total_cost":        "cost",
	"amount":            "cost",
	"provider":          "provider",
	"service":           "provider",
	"vendor":            "provider",
}

func detectColumns(rows [][]string) (int, columnMap) {
	cm := columnMap{model: -1, provider: -1, inputTokens: -1, outputTokens: -1, cost: -1}
	if len(rows) == 0 {
		return 0, cm
	}

	header := rows[0]
	found := 0
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		field, ok := knownHeaders[key]
		if !ok {
			continue
		}
		switch field {
		case "model":
			if cm.model < 0 { cm.model = i; found++ }
		case "input_tokens":
			if cm.inputTokens < 0 { cm.inputTokens = i; found++ }
		case "output_tokens":
			if cm.outputTokens < 0 { cm.outputTokens = i; found++ }
		case "cost":
			if cm.cost < 0 { cm.cost = i; found++ }
		case "provider":
			if cm.provider < 0 { cm.provider = i; found++ }
		}
	}

	if found >= 3 {
		return 1, cm
	}

	if len(rows[0]) >= 4 {
		return 0, columnMap{
			model:        0,
			provider:    -1,
			inputTokens: 1,
			outputTokens: 2,
			cost:        3,
		}
	}
	return 0, cm
}

func getCol(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func stripBOM(rows [][]string) [][]string {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return rows
	}
	rows[0][0] = strings.TrimLeft(rows[0][0], "\ufeff\u00a0\u200b")
	return rows
}

type parseError struct {
	Row     int
	Field   string
	Value   string
	Message string
}

func (e parseError) Error() string {
	return fmt.Sprintf("row %d, field %q: %s (value: %q)", e.Row, e.Field, e.Message, e.Value)
}

type modelUsageEntry struct {
	model    string
	provider string
	input    int64
	output   int64
	cost     float64
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse upload", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field 'file' required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read CSV: %v", err), http.StatusBadRequest)
		return
	}
	if len(rows) == 0 {
		http.Error(w, "empty CSV", http.StatusBadRequest)
		return
	}

	rows = stripBOM(rows)

	startRow, cm := detectColumns(rows)
	if cm.model < 0 {
		http.Error(w, "unable to detect columns. CSV must have columns: model, input_tokens, output_tokens, cost", http.StatusBadRequest)
		return
	}

	var parseErrors []parseError
	modelUsage := make(map[string]*engine.ModelUsage)
	modelProvider := make(map[string]string)
	totalCost := 0.0
	providerSet := make(map[string]bool)
	var totalInput, totalOutput int64
	rowCount := 0

	for i := startRow; i < len(rows); i++ {
		model := getCol(rows[i], cm.model)
		if model == "" {
			continue
		}

		inputStr := getCol(rows[i], cm.inputTokens)
		outputStr := getCol(rows[i], cm.outputTokens)
		costStr := getCol(rows[i], cm.cost)

		inputTokens, err := strconv.ParseInt(inputStr, 10, 64)
		if err != nil {
			parseErrors = append(parseErrors, parseError{Row: i + 1, Field: "input_tokens", Value: inputStr, Message: "invalid integer"})
			continue
		}
		outputTokens, err := strconv.ParseInt(outputStr, 10, 64)
		if err != nil {
			parseErrors = append(parseErrors, parseError{Row: i + 1, Field: "output_tokens", Value: outputStr, Message: "invalid integer"})
			continue
		}
		cost, err := strconv.ParseFloat(costStr, 64)
		if err != nil {
			parseErrors = append(parseErrors, parseError{Row: i + 1, Field: "cost", Value: costStr, Message: "invalid number"})
			continue
		}

		provider := getCol(rows[i], cm.provider)
		if provider == "" {
			provider = "imported"
		}

		totalCost += cost
		totalInput += inputTokens
		totalOutput += outputTokens
		providerSet[provider] = true
		rowCount++

		if existing, ok := modelUsage[model]; ok {
			existing.InputTokens += inputTokens
			existing.OutputTokens += outputTokens
			existing.RequestCount++
			existing.ActualCost += cost
		} else {
			modelUsage[model] = &engine.ModelUsage{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				RequestCount: 1,
				ActualCost:   cost,
			}
		}
		modelProvider[model+"\x00"+provider] = model
	}

	if rowCount == 0 {
		if len(parseErrors) > 0 {
			msg := fmt.Sprintf("no valid data rows found (%d parse errors)", len(parseErrors))
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		http.Error(w, "no valid data rows found in CSV", http.StatusBadRequest)
		return
	}

	var providersUsed []engine.ProviderUsage
	for p := range providerSet {
		var pModels []string
		for key := range modelProvider {
			parts := strings.SplitN(key, "\x00", 2)
			if len(parts) == 2 && parts[1] == p {
				pModels = append(pModels, parts[0])
			}
		}
		providersUsed = append(providersUsed, engine.ProviderUsage{
			Name:   p,
			Models: pModels,
		})
	}

	inputPct := 0.7
	if totalInput+totalOutput > 0 {
		inputPct = float64(totalInput) / float64(totalInput+totalOutput)
	}

	a := &engine.Assessment{
		CompanyName:          "CSV Upload Analysis",
		Source:               "live",
		CurrentMonthlySpend:  totalCost,
		MonthlyRequestVolume: int64(rowCount),
		TokenDistribution: engine.TokenDistribution{
			InputPct:  inputPct,
			OutputPct: 1 - inputPct,
		},
		ProvidersUsed: providersUsed,
		Currency:      "USD",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	assessmentID := store.AddAssessment(a)

	store.SetLiveData(&engine.AssessmentLiveData{
		TotalMonthlyCost: totalCost,
		Models:           modelUsage,
	})

	if _, err := engine.RunAssessment(store, assessmentID); err != nil {
		http.Error(w, fmt.Sprintf("assessment failed: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/report/%d", assessmentID), http.StatusSeeOther)
}

func handleReport(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/report/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if len(parts) > 1 && parts[1] == "pdf" {
		handlePDF(w, id)
		return
	}

	report, err := engine.GetReport(store, id)
	if err != nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	parseTmpl("report.html").Execute(w, ReportPage{Report: report, ID: id})
}

type importRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	OrgID    string `json:"org_id,omitempty"`
}

type importResult struct {
	Models    map[string]*engine.ModelUsage
	TotalCost float64
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		http.Error(w, "api_key field required", http.StatusBadRequest)
		return
	}

	var usageData *importResult
	var err error
	switch strings.ToLower(req.Provider) {
	case "openai":
		usageData, err = fetchOpenAIUsage(req.APIKey)
	case "anthropic":
		usageData, err = fetchAnthropicUsage(req.APIKey, req.OrgID)
	default:
		http.Error(w, "unsupported provider (supported: openai, anthropic)", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch billing data: %v", err), http.StatusBadGateway)
		return
	}

	var providersUsed []engine.ProviderUsage
	providerSet := make(map[string]bool)
	modelProvider := make(map[string]string)
	for model, usage := range usageData.Models {
		mi := engine.FindModel(model)
		provider := "imported"
		if mi != nil {
			provider = mi.Provider
		}
		providerSet[provider] = true
		modelProvider[model+"\x00"+provider] = model
		_ = usage
	}
	for p := range providerSet {
		var pModels []string
		for key := range modelProvider {
			parts := strings.SplitN(key, "\x00", 2)
			if len(parts) == 2 && parts[1] == p {
				pModels = append(pModels, parts[0])
			}
		}
		providersUsed = append(providersUsed, engine.ProviderUsage{
			Name:   p,
			Models: pModels,
		})
	}

	a := &engine.Assessment{
		CompanyName:         "API Key Import (" + req.Provider + ")",
		Source:              "live",
		CurrentMonthlySpend: usageData.TotalCost,
		ProvidersUsed:       providersUsed,
		Currency:            "USD",
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	}

	assessmentID := store.AddAssessment(a)

	store.SetLiveData(&engine.AssessmentLiveData{
		TotalMonthlyCost: usageData.TotalCost,
		Models:           usageData.Models,
	})

	if _, err := engine.RunAssessment(store, assessmentID); err != nil {
		http.Error(w, fmt.Sprintf("assessment failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"redirect_url": fmt.Sprintf("/report/%d", assessmentID),
		"assessment_id": assessmentID,
	})
}

func fetchOpenAIUsage(apiKey string) (*importResult, error) {
	end := time.Now().UTC()
	start := end.Add(-30 * 24 * time.Hour)
	url := fmt.Sprintf("https://api.openai.com/v1/dashboard/billing/usage?start_date=%s&end_date=%s",
		start.Format("2006-01-02"), end.Format("2006-01-02"))

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai api request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(body))
	}

	var raw struct {
		DailyCosts []json.RawMessage `json:"daily_costs"`
		TotalUsage float64            `json:"total_usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse openai response: %w", err)
	}

	type lineItem struct {
		Name string          `json:"name"`
		Cost json.RawMessage `json:"cost"`
	}
	type dailyCost struct {
		Timestamp int64      `json:"timestamp"`
		LineItems []lineItem `json:"line_items"`
	}

	result := &importResult{
		Models:    make(map[string]*engine.ModelUsage),
		TotalCost: raw.TotalUsage,
	}

	for _, dcRaw := range raw.DailyCosts {
		var dc dailyCost
		if err := json.Unmarshal(dcRaw, &dc); err != nil {
			continue
		}
		for _, li := range dc.LineItems {
			var costVal float64
			if err := json.Unmarshal(li.Cost, &costVal); err != nil {
				var costObj struct {
					Amount float64 `json:"amount"`
				}
				if err := json.Unmarshal(li.Cost, &costObj); err != nil {
					continue
				}
				costVal = costObj.Amount
			}
			modelName := normalizeModelName(li.Name)
			if existing, ok := result.Models[modelName]; ok {
				existing.ActualCost += costVal
				existing.RequestCount++
			} else {
				result.Models[modelName] = &engine.ModelUsage{
					RequestCount: 1,
					ActualCost:   costVal,
				}
			}
		}
	}

	if result.TotalCost <= 0 {
		for _, mu := range result.Models {
			result.TotalCost += mu.ActualCost
		}
	}

	return result, nil
}

func fetchAnthropicUsage(apiKey, orgID string) (*importResult, error) {
	baseURL := "https://api.anthropic.com/v1/organizations"
	if orgID != "" {
		baseURL += "/" + orgID + "/billing"
	} else {
		baseURL = "https://api.anthropic.com/v1/usage"
	}

	req, _ := http.NewRequest("GET", baseURL, nil)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic api request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Data []struct {
			Model        string  `json:"model"`
			InputTokens  int64   `json:"input_tokens"`
			OutputTokens int64   `json:"output_tokens"`
			Cost         float64 `json:"cost,omitempty"`
		} `json:"data,omitempty"`
		Bills []struct {
			LineItems []struct {
				Model        string `json:"model"`
				InputTokens  int64  `json:"input_tokens"`
				OutputTokens int64  `json:"output_tokens"`
				Cost         float64 `json:"cost"`
			} `json:"line_items"`
		} `json:"bills,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse anthropic response: %w", err)
	}

	result := &importResult{
		Models: make(map[string]*engine.ModelUsage),
	}

	for _, bill := range raw.Bills {
		for _, li := range bill.LineItems {
			modelName := normalizeModelName(li.Model)
			if existing, ok := result.Models[modelName]; ok {
				existing.InputTokens += li.InputTokens
				existing.OutputTokens += li.OutputTokens
				existing.ActualCost += li.Cost
				existing.RequestCount++
			} else {
				result.Models[modelName] = &engine.ModelUsage{
					InputTokens:  li.InputTokens,
					OutputTokens: li.OutputTokens,
					RequestCount: 1,
					ActualCost:   li.Cost,
				}
			}
		}
	}

	for _, item := range raw.Data {
		modelName := normalizeModelName(item.Model)
		if existing, ok := result.Models[modelName]; ok {
			existing.InputTokens += item.InputTokens
			existing.OutputTokens += item.OutputTokens
			if item.Cost > 0 {
				existing.ActualCost += item.Cost
			}
			existing.RequestCount++
		} else {
			mu := &engine.ModelUsage{
				InputTokens:  item.InputTokens,
				OutputTokens: item.OutputTokens,
				RequestCount: 1,
			}
			if item.Cost > 0 {
				mu.ActualCost = item.Cost
			}
			result.Models[modelName] = mu
		}
	}

	if len(result.Models) == 0 {
		return nil, fmt.Errorf("no usage data found — Anthropic billing API may require an organization API key and org_id parameter")
	}

	for _, mu := range result.Models {
		result.TotalCost += mu.ActualCost
	}

	return result, nil
}

func normalizeModelName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	repl := strings.NewReplacer("-", "", "_", "", ".", "", " ", "")
	name = repl.Replace(name)
	for _, m := range engine.ModelCatalog {
		mn := repl.Replace(m.Name)
		if mn == name {
			return m.Name
		}
	}
	return name
}

func generatePDF(report *engine.AssessmentReport) ([]byte, error) {
	sym := report.CurrencySymbol
	if sym == "" {
		sym = "$"
	}

	m := maroto.New(config.NewBuilder().WithLeftMargin(15).WithRightMargin(15).Build())

	m.AddRows(row.New(20).Add(col.New(12).Add(text.New("TokenSentinel Prescriptive Assessment Report", props.Text{
		Style: fontstyle.Bold, Size: 18, Align: align.Center,
	}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New(fmt.Sprintf("Prepared for: %s", report.Assessment.CompanyName), props.Text{
		Size: 12, Align: align.Center,
	}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New(fmt.Sprintf("Report Date: %s", time.Now().UTC().Format("January 02, 2006")), props.Text{
		Size: 10, Align: align.Center,
	}))))
	m.AddRows(row.New(12).Add(col.New(12).Add(text.New("", props.Text{}))))

	if report.TotalCurrent > 0 {
		savingsRate := (report.TotalSavings / report.TotalCurrent) * 100
		m.AddRows(row.New(20).Add(col.New(12).Add(text.New(fmt.Sprintf("Total Potential Savings: %s%.0f/mo (%.0f%%)",
			sym, report.TotalSavings, savingsRate), props.Text{
			Style: fontstyle.Bold, Size: 14, Align: align.Center,
		}))))
		m.AddRows(row.New(12).Add(col.New(12).Add(text.New("", props.Text{}))))
	}

	m.AddRows(row.New(12).Add(col.New(12).Add(text.New("1. Executive Summary", props.Text{Style: fontstyle.Bold, Size: 14, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New(fmt.Sprintf("Current monthly AI infrastructure spend: %s%.2f.", sym, report.TotalCurrent), props.Text{Size: 10, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New(fmt.Sprintf("Projected monthly spend after optimizations: %s%.2f.", sym, report.TotalProjected), props.Text{Size: 10, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New(fmt.Sprintf("Potential savings: %s%.2f per month.", sym, report.TotalSavings), props.Text{Size: 10, Align: align.Left}))))
	m.AddRows(row.New(10).Add(col.New(12).Add(text.New("", props.Text{}))))

	if len(report.Recommendations) > 0 {
		m.AddRows(row.New(12).Add(col.New(12).Add(text.New("2. Recommendations", props.Text{Style: fontstyle.Bold, Size: 14, Align: align.Left}))))
		for i, r := range report.Recommendations {
			if i > 0 {
				m.AddRows(row.New(4).Add(col.New(12).Add(text.New("", props.Text{}))))
			}
			m.AddRows(row.New(10).Add(
				col.New(1).Add(text.New(fmt.Sprintf("%d.", i+1), props.Text{Style: fontstyle.Bold, Size: 10, Align: align.Right})),
				col.New(2).Add(text.New(strings.ToUpper(r.Priority), props.Text{Style: fontstyle.Bold, Size: 9, Align: align.Left})),
				col.New(9).Add(text.New(fmt.Sprintf("%s%.0f/mo savings", sym, r.MonthlySavings), props.Text{Size: 10, Align: align.Left})),
			))
			m.AddRows(row.New(14).Add(
				col.New(1).Add(text.New("", props.Text{Size: 8})),
				col.New(11).Add(text.New(r.Description, props.Text{Size: 10, Align: align.Left})),
			))
			m.AddRows(row.New(8).Add(
				col.New(1).Add(text.New("", props.Text{Size: 8})),
				col.New(11).Add(text.New(fmt.Sprintf("Category: %s | Current spend: %s%.0f/mo | Payback: %d days",
					strings.ReplaceAll(r.Category, "_", " "), sym, r.CurrentCost, r.PaybackPeriodDays), props.Text{Size: 8, Align: align.Left})),
			))
		}
		m.AddRows(row.New(10).Add(col.New(12).Add(text.New("", props.Text{}))))
	}

	if len(report.CostBreakdown) > 0 {
		m.AddRows(row.New(12).Add(col.New(12).Add(text.New("3. Cost Breakdown by Model", props.Text{Style: fontstyle.Bold, Size: 14, Align: align.Left}))))
		m.AddRows(row.New(10).Add(
			col.New(3).Add(text.New("Model", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Left})),
			col.New(2).Add(text.New("Provider", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Left})),
			col.New(2).Add(text.New("Input (M)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right, Right: 3})),
			col.New(2).Add(text.New("Output (M)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right, Right: 3})),
			col.New(2).Add(text.New("Cur/Mo", props.Text{Style: fontstyle.Bold, Size: 7, Align: align.Right, Right: 2})),
			col.New(1).Add(text.New("Proj", props.Text{Style: fontstyle.Bold, Size: 7, Align: align.Right, Right: 2})),
		))
		for _, cp := range report.CostBreakdown {
			m.AddRows(row.New(8).Add(
				col.New(3).Add(text.New(cp.Model, props.Text{Size: 8, Align: align.Left})),
				col.New(2).Add(text.New(cp.Provider, props.Text{Size: 8, Align: align.Left})),
				col.New(2).Add(text.New(fmt.Sprintf("%.2f", cp.InputTokensMillions), props.Text{Size: 8, Align: align.Right, Right: 3})),
				col.New(2).Add(text.New(fmt.Sprintf("%.2f", cp.OutputTokensMillions), props.Text{Size: 8, Align: align.Right, Right: 3})),
				col.New(2).Add(text.New(fmt.Sprintf("%s%.0f", sym, cp.CurrentMonthlyCost), props.Text{Size: 7, Align: align.Right, Right: 2})),
				col.New(1).Add(text.New(fmt.Sprintf("%s%.0f", sym, cp.ProjectedMonthlyCost), props.Text{Size: 7, Align: align.Right, Right: 2})),
			))
		}
		m.AddRows(row.New(10).Add(col.New(12).Add(text.New("", props.Text{}))))
	}

	m.AddRows(row.New(12).Add(col.New(12).Add(text.New("4. Next Steps", props.Text{Style: fontstyle.Bold, Size: 14, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New("1. Review each recommendation with your engineering team.", props.Text{Size: 10, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New("2. Start with high-priority items offering the fastest payback.", props.Text{Size: 10, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New("3. Use what-if scenarios to model additional changes.", props.Text{Size: 10, Align: align.Left}))))
	m.AddRows(row.New(8).Add(col.New(12).Add(text.New("4. Re-run this assessment after implementing changes.", props.Text{Size: 10, Align: align.Left}))))

	m.AddRows(row.New(20).Add(col.New(12).Add(text.New(fmt.Sprintf("TokenSentinel — %d", time.Now().UTC().Year()), props.Text{
		Size: 8, Align: align.Center,
	}))))

	document, err := m.Generate()
	if err != nil {
		return nil, err
	}

	return document.GetBytes(), nil
}

func handlePDF(w http.ResponseWriter, assessmentID int) {
	report, err := engine.GetReport(store, assessmentID)
	if err != nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	pdfBytes, err := generatePDF(report)
	if err != nil {
		log.Printf("pdf generation error: %v", err)
		http.Error(w, "pdf generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=TokenSentinel_Assessment_%d_Report.pdf", assessmentID))
	w.Write(pdfBytes)
}
