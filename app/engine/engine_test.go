package engine

import (
	"testing"
	"time"
)

func newTestStore() *MemStore {
	return NewMemStore()
}

func TestRunAssessmentBasic(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "TestCorp",
		Source:               "manual",
		CurrentMonthlySpend:  10000,
		MonthlyRequestVolume: 100000,
		TokenDistribution: TokenDistribution{
			InputPct:  0.7,
			OutputPct: 0.3,
		},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4", "gpt-4o"}, MonthlySpend: 10000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	report, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	if report.TotalCurrent <= 0 {
		t.Errorf("expected positive TotalCurrent, got %f", report.TotalCurrent)
	}

	if len(report.CostBreakdown) == 0 {
		t.Error("expected at least one CostBreakdown entry")
	}

	if len(report.Recommendations) == 0 {
		t.Error("expected at least one Recommendation")
	}

	if report.TotalSavings < 0 {
		t.Errorf("TotalSavings should not be negative, got %f", report.TotalSavings)
	}
}

func TestRunAssessmentWithLiveData(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "LiveTest",
		Source:               "live",
		CurrentMonthlySpend:  0,
		MonthlyRequestVolume: 50,
		TokenDistribution: TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4"}, MonthlySpend: 0},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	s.SetLiveData(&AssessmentLiveData{
		TotalMonthlyCost: 5000,
		Models: map[string]*ModelUsage{
			"gpt-4": {InputTokens: 1000000, OutputTokens: 200000, RequestCount: 10, ActualCost: 4500},
		},
	})

	report, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	if report.TotalCurrent <= 0 {
		t.Errorf("expected positive TotalCurrent from live data, got %f", report.TotalCurrent)
	}

	if len(report.CostBreakdown) == 0 {
		t.Error("expected cost breakdown from live data")
	}

	if report.CostBreakdown[0].Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", report.CostBreakdown[0].Model)
	}
}

func TestCostBreakdownMultipleProviders(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName: "MultiProv",
		Source:      "manual",
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4", "gpt-4o-mini"}, MonthlySpend: 8000},
			{Name: "anthropic", Models: []string{"claude-3-opus"}, MonthlySpend: 5000},
		},
		MonthlyRequestVolume: 200000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		CurrentMonthlySpend:  13000,
		Currency:             "USD",
		CreatedAt:            time.Now().UTC().Format(time.RFC3339),
	})

	report, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	providers := make(map[string]bool)
	for _, cp := range report.CostBreakdown {
		providers[cp.Provider] = true
	}

	if !providers["openai"] {
		t.Error("expected openai in cost breakdown")
	}
	if !providers["anthropic"] {
		t.Error("expected anthropic in cost breakdown")
	}

	if len(report.Recommendations) == 0 {
		t.Error("expected recommendations from multi-provider setup")
	}
}

func TestRecommendationProviderSwitch(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "SwitchTest",
		Source:               "manual",
		CurrentMonthlySpend:  5000,
		MonthlyRequestVolume: 50000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4"}, MonthlySpend: 5000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	report, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	if len(report.Recommendations) == 0 {
		t.Error("expected at least one recommendation")
	}
}

func TestRecommendationBatchOptimization(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "BatchTest",
		Source:               "manual",
		CurrentMonthlySpend:  20000,
		MonthlyRequestVolume: 500000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4", "gpt-4o"}, MonthlySpend: 20000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	report, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	var found bool
	for _, r := range report.Recommendations {
		if r.Category == "batch_optimization" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected batch_optimization recommendation for high-volume workload")
	}
}

func TestRunAssessmentInvalidID(t *testing.T) {
	s := newTestStore()
	_, err := RunAssessment(s, 999)
	if err == nil {
		t.Error("expected error for non-existent assessment ID")
	}
}

func TestGetReport(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "ReportTest",
		Source:               "manual",
		CurrentMonthlySpend:  5000,
		MonthlyRequestVolume: 50000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4o-mini"}, MonthlySpend: 5000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	_, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	report, err := GetReport(s, id)
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}

	if report.TotalCurrent <= 0 {
		t.Errorf("expected positive TotalCurrent, got %f", report.TotalCurrent)
	}

	if report.CurrencySymbol != "$" {
		t.Errorf("expected $ symbol, got %s", report.CurrencySymbol)
	}
}

func TestRunWhatIf(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "WhatIf",
		Source:               "manual",
		CurrentMonthlySpend:  10000,
		MonthlyRequestVolume: 100000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4"}, MonthlySpend: 10000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	_, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	projections, err := RunWhatIf(s, id, map[string]float64{"volume_multiplier": 2.0})
	if err != nil {
		t.Fatalf("RunWhatIf failed: %v", err)
	}

	if len(projections) == 0 {
		t.Fatal("expected projections from what-if")
	}

	for _, p := range projections {
		if p.Scenario != "whatif" {
			t.Errorf("expected scenario 'whatif', got %s", p.Scenario)
		}
	}
}

func TestRunWhatIfInputPct(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "WhatIfPct",
		Source:               "manual",
		CurrentMonthlySpend:  5000,
		MonthlyRequestVolume: 50000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4"}, MonthlySpend: 5000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	_, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	projections, err := RunWhatIf(s, id, map[string]float64{"input_pct": 0.5})
	if err != nil {
		t.Fatalf("RunWhatIf failed: %v", err)
	}

	if len(projections) == 0 {
		t.Fatal("expected projections")
	}
}

func TestFindModel(t *testing.T) {
	mi := FindModel("gpt-4")
	if mi == nil {
		t.Fatal("expected to find gpt-4")
	}
	if mi.Provider != "openai" {
		t.Errorf("expected openai provider, got %s", mi.Provider)
	}
	if mi.Tier != TierFrontier {
		t.Errorf("expected TierFrontier for gpt-4, got %d", mi.Tier)
	}

	mi = FindModel("nonexistent")
	if mi != nil {
		t.Errorf("expected nil for unknown model, got %v", mi)
	}
}

func TestCurrencySymbol(t *testing.T) {
	if sym := CurrencySymbol("USD"); sym != "$" {
		t.Errorf("expected $, got %s", sym)
	}
	if sym := CurrencySymbol("INR"); sym != "₹" {
		t.Errorf("expected ₹, got %s", sym)
	}
	if sym := CurrencySymbol("UNKNOWN"); sym != "$" {
		t.Errorf("expected $ fallback, got %s", sym)
	}
}

func TestFXRate(t *testing.T) {
	if rate := FXRate("USD"); rate != 1.0 {
		t.Errorf("expected 1.0 for USD, got %f", rate)
	}
	if rate := FXRate("EUR"); rate != 0.92 {
		t.Errorf("expected 0.92 for EUR, got %f", rate)
	}
	if rate := FXRate("UNKNOWN"); rate != 1.0 {
		t.Errorf("expected 1.0 fallback, got %f", rate)
	}
}

func TestFromUSD(t *testing.T) {
	if v := FromUSD(100, "INR"); v != 8350 {
		t.Errorf("expected 8350 INR for $100, got %f", v)
	}
}

func TestToUSD(t *testing.T) {
	if v := ToUSD(8350, "INR"); v != 100 {
		t.Errorf("expected $100 for 8350 INR, got %f", v)
	}
}

func TestGetRoutingRules(t *testing.T) {
	rules := GetRoutingRules([]string{"gpt-4", "gpt-4o-mini"})
	for _, r := range rules {
		if r.Model != "" && r.SavingsPercent <= 0 {
			t.Errorf("expected positive savings for %s, got %f", r.Model, r.SavingsPercent)
		}
	}
}

func TestGetRoutingRulesCheapModel(t *testing.T) {
	rules := GetRoutingRules([]string{"gpt-3.5-turbo"})
	for _, r := range rules {
		t.Errorf("expected no rules for cheap model, got rule for %s", r.Model)
	}
}

func TestGetBudgetRoutingSuggestions(t *testing.T) {
	suggestions := GetBudgetRoutingSuggestions(map[string]float64{"gpt-4": 5000, "gpt-4o-mini": 1000})
	for _, s := range suggestions {
		if s.EstimatedSavings <= 0 {
			t.Errorf("expected positive savings for %s, got %f", s.Model, s.EstimatedSavings)
		}
	}
}

func TestCompareProjections(t *testing.T) {
	s := newTestStore()
	id := s.AddAssessment(&Assessment{
		CompanyName:          "CompareTest",
		Source:               "manual",
		CurrentMonthlySpend:  5000,
		MonthlyRequestVolume: 50000,
		TokenDistribution:    TokenDistribution{InputPct: 0.7, OutputPct: 0.3},
		ProvidersUsed: []ProviderUsage{
			{Name: "openai", Models: []string{"gpt-4o-mini"}, MonthlySpend: 5000},
		},
		Currency:  "USD",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	_, err := RunAssessment(s, id)
	if err != nil {
		t.Fatalf("RunAssessment failed: %v", err)
	}

	entries, err := CompareProjections(s, id, map[string]float64{"gpt-4o-mini": 100})
	if err != nil {
		t.Fatalf("CompareProjections failed: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected variance entries")
	}

	validDirections := map[string]bool{"under": true, "over": true, "on_track": true}
	if !validDirections[entries[0].Direction] {
		t.Errorf("unexpected variance direction: %s", entries[0].Direction)
	}
}

func TestCompareProjectionsNoData(t *testing.T) {
	s := newTestStore()
	_, err := CompareProjections(s, 999, map[string]float64{})
	if err == nil {
		t.Error("expected error for non-existent assessment")
	}
}

func TestGPUReference(t *testing.T) {
	g := FindGPUReference("A100")
	if g == nil {
		t.Fatal("expected to find A100")
	}
	if g.HourlyPrice != 3.50 {
		t.Errorf("expected 3.50, got %f", g.HourlyPrice)
	}

	g = FindGPUReference("nonexistent")
	if g != nil {
		t.Errorf("expected nil for unknown GPU, got %v", g)
	}
}

func TestEffectiveCurrency(t *testing.T) {
	a := &Assessment{Currency: "EUR"}
	if c := a.EffectiveCurrency(); c != "EUR" {
		t.Errorf("expected EUR, got %s", c)
	}

	a = &Assessment{}
	if c := a.EffectiveCurrency(); c != "USD" {
		t.Errorf("expected USD fallback, got %s", c)
	}
}

func TestEffectiveFXRate(t *testing.T) {
	a := &Assessment{Currency: "INR"}
	if rate := a.EffectiveFXRate(); rate != 83.50 {
		t.Errorf("expected 83.50 for INR, got %f", rate)
	}

	a = &Assessment{FXRate: 2.5}
	if rate := a.EffectiveFXRate(); rate != 2.5 {
		t.Errorf("expected 2.5 custom rate, got %f", rate)
	}
}
