package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type GPUReference struct {
	Type         string  `json:"type"`
	Description  string  `json:"description"`
	HourlyPrice  float64 `json:"hourly_price"`
	MonthlyPrice float64 `json:"monthly_price"`
	Tier         string  `json:"tier"` // datacenter, prosumer
}

var gpuReferencePricing = []GPUReference{
	{Type: "A100", Description: "NVIDIA A100 80GB PCIe", HourlyPrice: 3.50, MonthlyPrice: 2520, Tier: "datacenter"},
	{Type: "H100", Description: "NVIDIA H100 80GB SXM", HourlyPrice: 4.50, MonthlyPrice: 3240, Tier: "datacenter"},
	{Type: "H200", Description: "NVIDIA H200 141GB SXM", HourlyPrice: 5.00, MonthlyPrice: 3600, Tier: "datacenter"},
	{Type: "B100", Description: "NVIDIA B100 192GB", HourlyPrice: 6.00, MonthlyPrice: 4320, Tier: "datacenter"},
	{Type: "L40S", Description: "NVIDIA L40S 48GB", HourlyPrice: 2.50, MonthlyPrice: 1800, Tier: "datacenter"},
	{Type: "RTX4090", Description: "NVIDIA RTX 4090 24GB", HourlyPrice: 0.50, MonthlyPrice: 360, Tier: "prosumer"},
	{Type: "RTX5090", Description: "NVIDIA RTX 5090 32GB", HourlyPrice: 0.75, MonthlyPrice: 540, Tier: "prosumer"},
}

type ModelTier int

const (
	TierFrontier ModelTier = iota + 1
	TierCapable
	TierFast
	TierCheap
)

type ModelInfo struct {
	Name         string
	Provider     string
	Tier         ModelTier
	InputPrice   float64
	OutputPrice  float64
}

var modelCatalog = []ModelInfo{
	{Name: "gpt-4", Provider: "openai", Tier: TierFrontier, InputPrice: 30.00, OutputPrice: 60.00},
	{Name: "gpt-4-turbo", Provider: "openai", Tier: TierFrontier, InputPrice: 10.00, OutputPrice: 30.00},
	{Name: "gpt-4o", Provider: "openai", Tier: TierCapable, InputPrice: 2.50, OutputPrice: 10.00},
	{Name: "gpt-4o-mini", Provider: "openai", Tier: TierFast, InputPrice: 0.15, OutputPrice: 0.60},
	{Name: "gpt-3.5-turbo", Provider: "openai", Tier: TierCheap, InputPrice: 0.50, OutputPrice: 1.50},
	{Name: "claude-3-opus", Provider: "anthropic", Tier: TierFrontier, InputPrice: 15.00, OutputPrice: 75.00},
	{Name: "claude-3-sonnet", Provider: "anthropic", Tier: TierCapable, InputPrice: 3.00, OutputPrice: 15.00},
	{Name: "claude-3-haiku", Provider: "anthropic", Tier: TierFast, InputPrice: 0.25, OutputPrice: 1.25},
	{Name: "gemini-1.5-pro", Provider: "google", Tier: TierFrontier, InputPrice: 1.25, OutputPrice: 5.00},
	{Name: "gemini-1.5-flash", Provider: "google", Tier: TierCapable, InputPrice: 0.075, OutputPrice: 0.30},
	{Name: "mistral-large", Provider: "mistral", Tier: TierFrontier, InputPrice: 2.00, OutputPrice: 6.00},
	{Name: "mistral-small", Provider: "mistral", Tier: TierFast, InputPrice: 0.60, OutputPrice: 1.80},
	{Name: "llama-3-70b", Provider: "self-hosted", Tier: TierCapable, InputPrice: 0.59, OutputPrice: 0.79},
	{Name: "llama-3-8b", Provider: "self-hosted", Tier: TierCheap, InputPrice: 0.05, OutputPrice: 0.20},
	{Name: "mixtral-8x7b", Provider: "self-hosted", Tier: TierCapable, InputPrice: 0.24, OutputPrice: 0.72},
}

// Equivalence groups map each model to alternatives in the same tier
var modelEquivalence = map[string][]string{
	"gpt-4":         {"claude-3-opus", "gemini-1.5-pro", "mistral-large"},
	"gpt-4-turbo":   {"claude-3-opus", "gemini-1.5-pro"},
	"gpt-4o":        {"claude-3-sonnet", "gemini-1.5-flash", "llama-3-70b", "mixtral-8x7b"},
	"gpt-4o-mini":   {"claude-3-haiku", "mistral-small"},
	"gpt-3.5-turbo": {"llama-3-8b", "claude-3-haiku"},
	"claude-3-opus": {"gpt-4", "gemini-1.5-pro", "mistral-large"},
	"claude-3-sonnet": {"gpt-4o", "gemini-1.5-flash", "mixtral-8x7b"},
	"claude-3-haiku": {"gpt-4o-mini", "mistral-small"},
	"gemini-1.5-pro": {"gpt-4", "claude-3-opus", "mistral-large"},
	"gemini-1.5-flash": {"gpt-4o", "claude-3-sonnet"},
	"mistral-large": {"gpt-4", "claude-3-opus", "gemini-1.5-pro"},
	"mistral-small": {"gpt-4o-mini", "claude-3-haiku"},
	"llama-3-70b": {"gpt-4o", "mixtral-8x7b"},
	"llama-3-8b": {"gpt-3.5-turbo"},
}

func findModel(name string) *ModelInfo {
	name = strings.ToLower(name)
	for _, m := range modelCatalog {
		if strings.EqualFold(m.Name, name) {
			return &m
		}
	}
	return nil
}

func RunAssessment(assessmentID int) (*AssessmentReport, error) {
	var a Assessment
	var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
	var createdAt, updatedAt time.Time

	err := db.QueryRow(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
		FROM assessments WHERE id = $1`, assessmentID).Scan(
		&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
		&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("assessment %d not found", assessmentID)
	}
	if err != nil {
		return nil, fmt.Errorf("query assessment %d: %w", assessmentID, err)
	}

	json.Unmarshal(gpuJSON, &a.GPUConfigs)
	json.Unmarshal(tokenJSON, &a.TokenDistribution)
	json.Unmarshal(providerJSON, &a.ProvidersUsed)
	json.Unmarshal(teamJSON, &a.TeamComposition)

	var liveData *AssessmentLiveData
	if a.Source == "live" {
		ld, err := pullLiveCostData(assessmentID)
		if err == nil && ld != nil {
			liveData = ld
			a.CurrentMonthlySpend = ld.TotalMonthlyCost
		}
	}

	costBreakdown := calculateCostBreakdown(&a, liveData)
	recommendations := generateRecommendations(&a, costBreakdown)
	totalCurrent := 0.0
	totalProjected := 0.0
	for _, c := range costBreakdown {
		totalCurrent += c.CurrentMonthlyCost
		totalProjected += c.ProjectedMonthlyCost
	}
	totalSavings := 0.0
	for _, r := range recommendations {
		totalSavings += r.MonthlySavings
	}

	db.Exec(`DELETE FROM cost_projections WHERE assessment_id = $1 AND scenario = 'base'`, assessmentID)
	for _, cp := range costBreakdown {
		db.Exec(`INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'base')`,
			assessmentID, cp.Model, cp.Provider, cp.CurrentMonthlyCost, cp.ProjectedMonthlyCost, cp.InputTokensMillions, cp.OutputTokensMillions)
	}
	db.Exec(`DELETE FROM recommendations WHERE assessment_id = $1`, assessmentID)
	for _, rec := range recommendations {
		db.Exec(`INSERT INTO recommendations (assessment_id, category, description, current_cost, projected_cost, monthly_savings, payback_period_days, priority)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			assessmentID, rec.Category, rec.Description, rec.CurrentCost, rec.ProjectedCost, rec.MonthlySavings, rec.PaybackPeriodDays, rec.Priority)
	}

	report := &AssessmentReport{
		Assessment:     a,
		CostBreakdown:  costBreakdown,
		Recommendations: recommendations,
		TotalCurrent:   totalCurrent,
		TotalProjected: totalProjected,
		TotalSavings:   totalSavings,
	}
	return report, nil
}

type AssessmentLiveData struct {
	TotalMonthlyCost float64
	Models           map[string]*ModelUsage
}

type ModelUsage struct {
	InputTokens  int64
	OutputTokens int64
	RequestCount int64
}

func pullLiveCostData(assessmentID int) (*AssessmentLiveData, error) {
	since := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	rows, err := db.Query(`SELECT model, SUM(input_tokens), SUM(output_tokens), COUNT(*)
		FROM cost_entries WHERE timestamp >= $1 GROUP BY model`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ld := &AssessmentLiveData{Models: make(map[string]*ModelUsage)}
	for rows.Next() {
		var model string
		var inputSum, outputSum, count int64
		if err := rows.Scan(&model, &inputSum, &outputSum, &count); err != nil {
			continue
		}
		ld.Models[model] = &ModelUsage{
			InputTokens:  inputSum,
			OutputTokens: outputSum,
			RequestCount: count,
		}
		inputCost := (float64(inputSum) / 1000) * 30.00
		outputCost := (float64(outputSum) / 1000) * 60.00
		if mi := findModel(model); mi != nil {
			inputCost = (float64(inputSum) / 1000) * mi.InputPrice
			outputCost = (float64(outputSum) / 1000) * mi.OutputPrice
		}
		ld.TotalMonthlyCost += inputCost + outputCost
	}
	return ld, nil
}

func calculateCostBreakdown(a *Assessment, liveData *AssessmentLiveData) []CostProjection {
	liveModels := make(map[string]bool)
	var projections []CostProjection

	if liveData != nil && len(liveData.Models) > 0 {
		for model, usage := range liveData.Models {
			liveModels[model] = true
			mi := findModel(model)
			provider := "unknown"
			inputPrice := 30.00
			outputPrice := 60.00
			if mi != nil {
				provider = mi.Provider
				inputPrice = mi.InputPrice
				outputPrice = mi.OutputPrice
			}
			currentCost := (float64(usage.InputTokens)/1000)*inputPrice + (float64(usage.OutputTokens)/1000)*outputPrice
			inputM := float64(usage.InputTokens) / 1_000_000
			outputM := float64(usage.OutputTokens) / 1_000_000
			projections = append(projections, CostProjection{
				Model:                model,
				Provider:             provider,
				CurrentMonthlyCost:   currentCost,
				ProjectedMonthlyCost: currentCost,
				InputTokensMillions:  inputM,
				OutputTokensMillions: outputM,
				Scenario:             "base",
			})
		}
	}

	if a.CurrentMonthlySpend > 0 && len(a.ProvidersUsed) > 0 {
		for _, pu := range a.ProvidersUsed {
			for _, model := range pu.Models {
				if liveModels[model] {
					continue
				}
				mi := findModel(model)
				provider := pu.Name
				inputPrice := 30.00
				outputPrice := 60.00
				if mi != nil {
					inputPrice = mi.InputPrice
					outputPrice = mi.OutputPrice
					provider = mi.Provider
				}
				avgPricePerK := (inputPrice + outputPrice) / 2
				modelFraction := 1.0 / float64(len(pu.Models))
				modelSpend := pu.MonthlySpend * modelFraction
				totalTokensK := modelSpend / avgPricePerK * 1000
				inputPct := 0.7
				if a.TokenDistribution.InputPct > 0 {
					inputPct = a.TokenDistribution.InputPct
				}
				inputTokensK := totalTokensK * inputPct
				outputTokensK := totalTokensK * (1 - inputPct)
				projections = append(projections, CostProjection{
					Model:                model,
					Provider:             provider,
					CurrentMonthlyCost:   modelSpend,
					ProjectedMonthlyCost: modelSpend,
					InputTokensMillions:  inputTokensK / 1000,
					OutputTokensMillions: outputTokensK / 1000,
					Scenario:             "base",
				})
			}
		}
	}

	return projections
}

func generateRecommendations(a *Assessment, costBreakdown []CostProjection) []Recommendation {
	var recs []Recommendation

	for _, cp := range costBreakdown {
		subRecs := recommendModelSubstitution(&cp)
		recs = append(recs, subRecs...)
	}

	if len(a.GPUConfigs) > 0 {
		infraRecs := recommendInfraDownsize(a)
		recs = append(recs, infraRecs...)
	}

	providerRecs := recommendProviderSwitch(costBreakdown)
	recs = append(recs, providerRecs...)

	batchRecs := recommendBatchOptimization(a, costBreakdown)
	recs = append(recs, batchRecs...)

	return recs
}

func recommendModelSubstitution(cp *CostProjection) []Recommendation {
	mi := findModel(cp.Model)
	if mi == nil || mi.Tier == TierCheap {
		return nil
	}

	var best *ModelInfo
	var bestSavings float64

	equivalents := modelEquivalence[mi.Name]
	for _, eqName := range equivalents {
		eq := findModel(eqName)
		if eq == nil {
			continue
		}
		if eq.Tier >= mi.Tier {
			continue
		}
		currentAvgPrice := (mi.InputPrice + mi.OutputPrice) / 2
		eqAvgPrice := (eq.InputPrice + eq.OutputPrice) / 2
		savings := cp.CurrentMonthlyCost * (1 - eqAvgPrice/currentAvgPrice)
		if savings > bestSavings {
			bestSavings = savings
			best = eq
		}
	}

	if best == nil || bestSavings < 10 {
		return nil
	}

	priority := "medium"
	if bestSavings > cp.CurrentMonthlyCost*0.5 {
		priority = "high"
	} else if bestSavings < cp.CurrentMonthlyCost*0.1 {
		priority = "low"
	}

	return []Recommendation{{
		Category:          "model_switch",
		Description:       fmt.Sprintf("Switch %s from %s to %s — save $%.0f/mo", cp.Model, mi.Provider, best.Provider, bestSavings),
		CurrentCost:       cp.CurrentMonthlyCost,
		ProjectedCost:     cp.CurrentMonthlyCost - bestSavings,
		MonthlySavings:    bestSavings,
		PaybackPeriodDays: 0,
		Priority:          priority,
	}}
}

func recommendInfraDownsize(a *Assessment) []Recommendation {
	var recs []Recommendation
	totalGPUs := 0
	for _, gpu := range a.GPUConfigs {
		totalGPUs += gpu.Count
	}
	if totalGPUs == 0 {
		return nil
	}

	estimatedReqPerGPU := float64(a.MonthlyRequestVolume) / float64(totalGPUs)
	if estimatedReqPerGPU < 50000 {
		reduction := int(math.Ceil(float64(totalGPUs) * 0.3))
		if reduction < 1 {
			reduction = 1
		}
		savingsPerGPU := 0.0
		for _, gpu := range a.GPUConfigs {
			if gpu.HourlyPrice > 0 {
				cost := gpu.HourlyPrice * float64(gpu.Count) * 730
				ratio := float64(gpu.Count) / float64(totalGPUs)
				savingsPerGPU += cost * ratio
			} else {
				ref := findGPUReference(gpu.Type)
				if ref != nil {
					cost := ref.HourlyPrice * float64(gpu.Count) * 730
					ratio := float64(gpu.Count) / float64(totalGPUs)
					savingsPerGPU += cost * ratio
				}
			}
		}
		monthlySavings := (float64(reduction) / float64(totalGPUs)) * savingsPerGPU
		if monthlySavings > 100 {
			recs = append(recs, Recommendation{
				Category:          "infra_downsize",
				Description:       fmt.Sprintf("Reduce GPU cluster from %d to ~%d nodes (%.0f%% utilization) — save $%.0f/mo", totalGPUs, totalGPUs-reduction, estimatedReqPerGPU/min(estimatedReqPerGPU*2, estimatedReqPerGPU+50000)*100, monthlySavings),
				CurrentCost:       savingsPerGPU,
				ProjectedCost:     savingsPerGPU - monthlySavings,
				MonthlySavings:    monthlySavings,
				PaybackPeriodDays: 0,
				Priority:          "medium",
			})
		}
	}
	return recs
}

func findGPUReference(gpuType string) *GPUReference {
	for _, g := range gpuReferencePricing {
		if strings.EqualFold(g.Type, gpuType) {
			return &g
		}
	}
	return nil
}

func recommendProviderSwitch(costBreakdown []CostProjection) []Recommendation {
	var recs []Recommendation
	providerCosts := make(map[string]float64)
	for _, cp := range costBreakdown {
		providerCosts[cp.Provider] += cp.CurrentMonthlyCost
	}

	for provider, cost := range providerCosts {
		if provider == "self-hosted" {
			continue
		}
		if cost < 500 {
			continue
		}
		mi := findModel(costBreakdown[0].Model)
		if mi == nil {
			continue
		}
		selfHostedAlt := findModel("llama-3-70b")
		if selfHostedAlt == nil {
			continue
		}
		savings := cost * 0.6
		if savings > 200 {
			recs = append(recs, Recommendation{
				Category:          "provider_switch",
				Description:       fmt.Sprintf("Move %s workloads ($%.0f/mo) to self-hosted Llama-3-70B — estimated savings $%.0f/mo", provider, cost, savings),
				CurrentCost:       cost,
				ProjectedCost:     cost - savings,
				MonthlySavings:    savings,
				PaybackPeriodDays: 30,
				Priority:          "medium",
			})
		}
	}
	return recs
}

func recommendBatchOptimization(a *Assessment, costBreakdown []CostProjection) []Recommendation {
	if a.MonthlyRequestVolume < 100000 {
		return nil
	}
	batchableFraction := 0.3
	offPeakDiscount := 0.5
	totalCost := 0.0
	for _, cp := range costBreakdown {
		totalCost += cp.CurrentMonthlyCost
	}
	savings := totalCost * batchableFraction * offPeakDiscount
	if savings < 50 {
		return nil
	}
	return []Recommendation{{
		Category:          "batch_optimization",
		Description:       fmt.Sprintf("Move %.0f%% of workloads to batch/off-peak (estimated %.0f%% discount) — save $%.0f/mo", batchableFraction*100, offPeakDiscount*100, savings),
		CurrentCost:       totalCost,
		ProjectedCost:     totalCost - savings,
		MonthlySavings:    savings,
		PaybackPeriodDays: 0,
		Priority:          "low",
	}}
}

func GetReport(assessmentID int) (*AssessmentReport, error) {
	var a Assessment
	var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
	var createdAt, updatedAt time.Time
	err := db.QueryRow(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
		FROM assessments WHERE id = $1`, assessmentID).Scan(
		&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
		&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("assessment %d not found", assessmentID)
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(gpuJSON, &a.GPUConfigs)
	json.Unmarshal(tokenJSON, &a.TokenDistribution)
	json.Unmarshal(providerJSON, &a.ProvidersUsed)
	json.Unmarshal(teamJSON, &a.TeamComposition)
	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)

	projRows, err := db.Query(`SELECT model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions
		FROM cost_projections WHERE assessment_id = $1 AND scenario = 'base'`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer projRows.Close()

	var projections []CostProjection
	for projRows.Next() {
		var cp CostProjection
		if err := projRows.Scan(&cp.Model, &cp.Provider, &cp.CurrentMonthlyCost, &cp.ProjectedMonthlyCost, &cp.InputTokensMillions, &cp.OutputTokensMillions); err != nil {
			continue
		}
		cp.Scenario = "base"
		cp.AssessmentID = assessmentID
		projections = append(projections, cp)
	}

	recRows, err := db.Query(`SELECT id, category, description, current_cost, projected_cost, monthly_savings, payback_period_days, priority, created_at
		FROM recommendations WHERE assessment_id = $1 ORDER BY
		CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, monthly_savings DESC`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer recRows.Close()

	var recs []Recommendation
	for recRows.Next() {
		var r Recommendation
		var createdAt time.Time
		if err := recRows.Scan(&r.ID, &r.Category, &r.Description, &r.CurrentCost, &r.ProjectedCost, &r.MonthlySavings, &r.PaybackPeriodDays, &r.Priority, &createdAt); err != nil {
			continue
		}
		r.AssessmentID = assessmentID
		r.CreatedAt = createdAt.Format(time.RFC3339)
		recs = append(recs, r)
	}

	totalCurrent := 0.0
	totalProjected := 0.0
	for _, cp := range projections {
		totalCurrent += cp.CurrentMonthlyCost
		totalProjected += cp.ProjectedMonthlyCost
	}
	totalSavings := totalCurrent - totalProjected
	if totalSavings < 0 {
		totalSavings = 0
	}
	for _, r := range recs {
		totalSavings += r.MonthlySavings
	}

	return &AssessmentReport{
		Assessment:      a,
		CostBreakdown:   projections,
		Recommendations: recs,
		TotalCurrent:    totalCurrent,
		TotalProjected:  totalProjected,
		TotalSavings:    totalSavings,
	}, nil
}

func RunWhatIf(assessmentID int, adjustments map[string]float64) ([]CostProjection, error) {
	var a Assessment
	var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
	err := db.QueryRow(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source, version
		FROM assessments WHERE id = $1`, assessmentID).Scan(
		&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
		&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("assessment %d not found", assessmentID)
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(gpuJSON, &a.GPUConfigs)
	json.Unmarshal(tokenJSON, &a.TokenDistribution)
	json.Unmarshal(providerJSON, &a.ProvidersUsed)
	json.Unmarshal(teamJSON, &a.TeamComposition)

	if v, ok := adjustments["volume_multiplier"]; ok {
		a.MonthlyRequestVolume = int64(float64(a.MonthlyRequestVolume) * v)
	}
	if v, ok := adjustments["input_pct"]; ok {
		a.TokenDistribution.InputPct = v
		a.TokenDistribution.OutputPct = 1 - v
	}

	costBreakdown := calculateCostBreakdown(&a, nil)
	for i := range costBreakdown {
		costBreakdown[i].Scenario = "whatif"
	}

	for _, cp := range costBreakdown {
		db.Exec(`INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'whatif')`,
			assessmentID, cp.Model, cp.Provider, cp.CurrentMonthlyCost, cp.ProjectedMonthlyCost, cp.InputTokensMillions, cp.OutputTokensMillions)
	}

	return costBreakdown, nil
}
