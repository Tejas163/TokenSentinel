package engine

import (
	"fmt"
	"math"
	"time"
)

type AssessmentLiveData struct {
	TotalMonthlyCost float64
	Models           map[string]*ModelUsage
}

type ModelUsage struct {
	InputTokens  int64
	OutputTokens int64
	RequestCount int64
}

func RunAssessment(store Store, assessmentID int) (*AssessmentReport, error) {
	a, err := store.GetAssessment(assessmentID)
	if err != nil {
		return nil, err
	}

	var liveData *AssessmentLiveData
	if a.Source == "live" {
		since := time.Now().UTC().Add(-30 * 24 * time.Hour)
		ld, err := store.QueryLiveCostData(since)
		if err == nil && ld != nil {
			liveData = ld
			a.CurrentMonthlySpend = ld.TotalMonthlyCost
		}
	}

	costBreakdown := calculateCostBreakdown(a, liveData)
	recommendations := generateRecommendations(a, costBreakdown)
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

	store.ReplaceCostProjections(assessmentID, costBreakdown)
	store.ReplaceRecommendations(assessmentID, recommendations)

	return &AssessmentReport{
		Assessment:      *a,
		CostBreakdown:   costBreakdown,
		Recommendations: recommendations,
		TotalCurrent:    totalCurrent,
		TotalProjected:  totalProjected,
		TotalSavings:    totalSavings,
	}, nil
}

func calculateCostBreakdown(a *Assessment, liveData *AssessmentLiveData) []CostProjection {
	liveModels := make(map[string]bool)
	var projections []CostProjection

	if liveData != nil && len(liveData.Models) > 0 {
		for model, usage := range liveData.Models {
			liveModels[model] = true
			mi := FindModel(model)
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
				mi := FindModel(model)
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
	mi := FindModel(cp.Model)
	if mi == nil || mi.Tier == TierCheap {
		return nil
	}

	var best *ModelInfo
	var bestSavings float64

	equivalents := ModelEquivalence[mi.Name]
	for _, eqName := range equivalents {
		eq := FindModel(eqName)
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
				ref := FindGPUReference(gpu.Type)
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
		selfHostedAlt := FindModel("llama-3-70b")
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

func GetReport(store Store, assessmentID int) (*AssessmentReport, error) {
	a, err := store.GetAssessment(assessmentID)
	if err != nil {
		return nil, err
	}

	projections, err := store.GetCostProjections(assessmentID, "base")
	if err != nil {
		return nil, err
	}

	recs, err := store.GetRecommendations(assessmentID)
	if err != nil {
		return nil, err
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
		Assessment:      *a,
		CostBreakdown:   projections,
		Recommendations: recs,
		TotalCurrent:    totalCurrent,
		TotalProjected:  totalProjected,
		TotalSavings:    totalSavings,
	}, nil
}

type RoutingRule struct {
	Model           string   `json:"model"`
	SuggestedTarget string   `json:"suggested_target"`
	Reason          string   `json:"reason"`
	CurrentPrice    float64  `json:"current_price_per_mtok"`
	TargetPrice     float64  `json:"target_price_per_mtok"`
	SavingsPercent  float64  `json:"savings_percent"`
	Confidence      string   `json:"confidence"`
	Tags            []string `json:"tags,omitempty"`
}

func GetRoutingRules(models []string) []RoutingRule {
	type candidate struct {
		model   *ModelInfo
		target  *ModelInfo
		savings float64
	}
	var rules []RoutingRule
	for _, name := range models {
		mi := FindModel(name)
		if mi == nil || mi.Tier == TierCheap {
			continue
		}
		var best candidate
		for _, eqName := range ModelEquivalence[name] {
			eq := FindModel(eqName)
			if eq == nil || eq.Tier >= mi.Tier {
				continue
			}
			currentAvg := (mi.InputPrice + mi.OutputPrice) / 2
			targetAvg := (eq.InputPrice + eq.OutputPrice) / 2
			savings := 1 - targetAvg/currentAvg
			if savings > best.savings {
				best = candidate{model: mi, target: eq, savings: savings}
			}
		}
		if best.target == nil || best.savings < 0.05 {
			continue
		}
		tags := []string{"cost_optimization"}
		if best.savings > 0.5 {
			tags = append(tags, "high_impact")
		}
		if mi.Provider != best.target.Provider {
			tags = append(tags, "provider_switch")
		}
		confidence := "medium"
		if best.savings > 0.4 {
			confidence = "high"
		} else if best.savings < 0.1 {
			confidence = "low"
		}
		rules = append(rules, RoutingRule{
			Model:           mi.Name,
			SuggestedTarget: best.target.Name,
			Reason:          fmt.Sprintf("Switch %s ($%.2f/Mtok) to %s ($%.2f/Mtok) — save %.0f%%", mi.Name, currentAvgPrice(mi), best.target.Name, currentAvgPrice(best.target), best.savings*100),
			CurrentPrice:    currentAvgPrice(mi),
			TargetPrice:     currentAvgPrice(best.target),
			SavingsPercent:  best.savings * 100,
			Confidence:      confidence,
			Tags:            tags,
		})
	}
	return rules
}

type BudgetRoutingSuggestion struct {
	Model            string  `json:"model"`
	CurrentProvider  string  `json:"current_provider"`
	SuggestedModel   string  `json:"suggested_model"`
	SuggestedProvider string `json:"suggested_provider"`
	EstimatedSavings float64 `json:"estimated_savings_per_mtok"`
	TierDrop         bool    `json:"tier_drop"`
}

func GetBudgetRoutingSuggestions(usageByModel map[string]float64) []BudgetRoutingSuggestion {
	var suggestions []BudgetRoutingSuggestion
	for model := range usageByModel {
		mi := FindModel(model)
		if mi == nil || mi.Tier == TierCheap {
			continue
		}
		var best *ModelInfo
		var bestSavings float64
		for _, eqName := range ModelEquivalence[model] {
			eq := FindModel(eqName)
			if eq == nil || eq.Tier >= mi.Tier {
				continue
			}
			currentAvg := (mi.InputPrice + mi.OutputPrice) / 2
			targetAvg := (eq.InputPrice + eq.OutputPrice) / 2
			savings := (currentAvg - targetAvg) / currentAvg
			if savings > bestSavings {
				bestSavings = savings
				best = eq
			}
		}
		if best == nil || bestSavings < 0.05 {
			continue
		}
		suggestions = append(suggestions, BudgetRoutingSuggestion{
			Model:             model,
			CurrentProvider:   mi.Provider,
			SuggestedModel:    best.Name,
			SuggestedProvider: best.Provider,
			EstimatedSavings:  bestSavings,
			TierDrop:          best.Tier < mi.Tier,
		})
	}
	return suggestions
}

type VarianceEntry struct {
	Model              string  `json:"model"`
	ProjectedCost      float64 `json:"projected_cost"`
	ActualCost         float64 `json:"actual_cost"`
	Variance           float64 `json:"variance"`
	VariancePercent    float64 `json:"variance_percent"`
	Direction          string  `json:"direction"` // under, over, on_track
}

func CompareProjections(store Store, assessmentID int, actualCosts map[string]float64) ([]VarianceEntry, error) {
	projections, err := store.GetCostProjections(assessmentID, "base")
	if err != nil {
		return nil, err
	}
	if len(projections) == 0 {
		return nil, fmt.Errorf("no projections found for assessment %d", assessmentID)
	}
	var entries []VarianceEntry
	for _, cp := range projections {
		actual := actualCosts[cp.Model]
		variance := actual - cp.ProjectedMonthlyCost
		variancePct := 0.0
		if cp.ProjectedMonthlyCost > 0 {
			variancePct = (variance / cp.ProjectedMonthlyCost) * 100
		}
		direction := "on_track"
		if variance > cp.ProjectedMonthlyCost*0.1 {
			direction = "over"
		} else if variance < -cp.ProjectedMonthlyCost*0.1 {
			direction = "under"
		}
		entries = append(entries, VarianceEntry{
			Model:           cp.Model,
			ProjectedCost:   cp.ProjectedMonthlyCost,
			ActualCost:      actual,
			Variance:        variance,
			VariancePercent: variancePct,
			Direction:       direction,
		})
	}
	return entries, nil
}

func currentAvgPrice(mi *ModelInfo) float64 {
	return (mi.InputPrice + mi.OutputPrice) / 2
}

func RunWhatIf(store Store, assessmentID int, adjustments map[string]float64) ([]CostProjection, error) {
	a, err := store.GetAssessment(assessmentID)
	if err != nil {
		return nil, err
	}

	if v, ok := adjustments["volume_multiplier"]; ok {
		a.MonthlyRequestVolume = int64(float64(a.MonthlyRequestVolume) * v)
	}
	if v, ok := adjustments["input_pct"]; ok {
		a.TokenDistribution.InputPct = v
		a.TokenDistribution.OutputPct = 1 - v
	}

	costBreakdown := calculateCostBreakdown(a, nil)
	for i := range costBreakdown {
		costBreakdown[i].Scenario = "whatif"
	}

	store.InsertCostProjections(assessmentID, costBreakdown, "whatif")

	return costBreakdown, nil
}
