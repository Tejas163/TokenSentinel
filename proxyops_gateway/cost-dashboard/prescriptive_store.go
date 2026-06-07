package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/proxyops/internal/engine"
)

type pgStore struct {
	db *sql.DB
}

func (s *pgStore) GetAssessment(id int) (*engine.Assessment, error) {
	var a engine.Assessment
	var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source,
		currency, fx_rate, version, created_at, updated_at
		FROM assessments WHERE id = $1`, id).Scan(
		&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
		&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source,
		&a.Currency, &a.FXRate, &a.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("assessment %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query assessment %d: %w", id, err)
	}

	if err := json.Unmarshal(gpuJSON, &a.GPUConfigs); err != nil {
		slog.Error("unmarshal gpu_configs for assessment", "id", id, "err", err)
	}
	if err := json.Unmarshal(tokenJSON, &a.TokenDistribution); err != nil {
		slog.Error("unmarshal token_distribution for assessment", "id", id, "err", err)
	}
	if err := json.Unmarshal(providerJSON, &a.ProvidersUsed); err != nil {
		slog.Error("unmarshal providers_used for assessment", "id", id, "err", err)
	}
	if err := json.Unmarshal(teamJSON, &a.TeamComposition); err != nil {
		slog.Error("unmarshal team_composition for assessment", "id", id, "err", err)
	}
	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &a, nil
}

func (s *pgStore) QueryLiveCostData(since time.Time) (*engine.AssessmentLiveData, error) {
	sinceStr := since.Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT model, SUM(input_tokens), SUM(output_tokens), COUNT(*)
		FROM cost_entries WHERE timestamp >= $1 GROUP BY model`, sinceStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ld := &engine.AssessmentLiveData{Models: make(map[string]*engine.ModelUsage)}
	for rows.Next() {
		var model string
		var inputSum, outputSum, count int64
		if err := rows.Scan(&model, &inputSum, &outputSum, &count); err != nil {
			slog.Error("scan live cost data", "err", err)
			continue
		}
		ld.Models[model] = &engine.ModelUsage{
			InputTokens:  inputSum,
			OutputTokens: outputSum,
			RequestCount: count,
		}
		inputCost := (float64(inputSum) / 1_000_000) * 30.00
		outputCost := (float64(outputSum) / 1_000_000) * 60.00
		if mi := engine.FindModel(model); mi != nil {
			inputCost = (float64(inputSum) / 1_000_000) * mi.InputPrice
			outputCost = (float64(outputSum) / 1_000_000) * mi.OutputPrice
		}
		ld.TotalMonthlyCost += inputCost + outputCost
	}
	return ld, nil
}

func (s *pgStore) ReplaceCostProjections(assessmentID int, projections []engine.CostProjection) error {
	s.db.Exec(`DELETE FROM cost_projections WHERE assessment_id = $1 AND scenario = 'base'`, assessmentID)
	for _, cp := range projections {
		_, err := s.db.Exec(`INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'base')`,
			assessmentID, cp.Model, cp.Provider, cp.CurrentMonthlyCost, cp.ProjectedMonthlyCost, cp.InputTokensMillions, cp.OutputTokensMillions)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *pgStore) ReplaceRecommendations(assessmentID int, recs []engine.Recommendation) error {
	s.db.Exec(`DELETE FROM recommendations WHERE assessment_id = $1`, assessmentID)
	for _, rec := range recs {
		_, err := s.db.Exec(`INSERT INTO recommendations (assessment_id, category, description, current_cost, projected_cost, monthly_savings, payback_period_days, priority)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			assessmentID, rec.Category, rec.Description, rec.CurrentCost, rec.ProjectedCost, rec.MonthlySavings, rec.PaybackPeriodDays, rec.Priority)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *pgStore) GetCostProjections(assessmentID int, scenario string) ([]engine.CostProjection, error) {
	rows, err := s.db.Query(`SELECT model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions
		FROM cost_projections WHERE assessment_id = $1 AND scenario = $2`, assessmentID, scenario)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projections []engine.CostProjection
	for rows.Next() {
		var cp engine.CostProjection
		if err := rows.Scan(&cp.Model, &cp.Provider, &cp.CurrentMonthlyCost, &cp.ProjectedMonthlyCost, &cp.InputTokensMillions, &cp.OutputTokensMillions); err != nil {
			slog.Error("scan cost projection", "err", err)
			continue
		}
		cp.Scenario = scenario
		cp.AssessmentID = assessmentID
		projections = append(projections, cp)
	}
	return projections, nil
}

func (s *pgStore) GetRecommendations(assessmentID int) ([]engine.Recommendation, error) {
	rows, err := s.db.Query(`SELECT id, category, description, current_cost, projected_cost, monthly_savings, payback_period_days, priority, created_at
		FROM recommendations WHERE assessment_id = $1 ORDER BY
		CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, monthly_savings DESC`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []engine.Recommendation
	for rows.Next() {
		var r engine.Recommendation
		var createdAt time.Time
		if err := rows.Scan(&r.ID, &r.Category, &r.Description, &r.CurrentCost, &r.ProjectedCost, &r.MonthlySavings, &r.PaybackPeriodDays, &r.Priority, &createdAt); err != nil {
			slog.Error("scan recommendation", "err", err)
			continue
		}
		r.AssessmentID = assessmentID
		r.CreatedAt = createdAt.Format(time.RFC3339)
		recs = append(recs, r)
	}
	return recs, nil
}

func (s *pgStore) InsertCostProjections(assessmentID int, projections []engine.CostProjection, scenario string) error {
	for _, cp := range projections {
		_, err := s.db.Exec(`INSERT INTO cost_projections (assessment_id, model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			assessmentID, cp.Model, cp.Provider, cp.CurrentMonthlyCost, cp.ProjectedMonthlyCost, cp.InputTokensMillions, cp.OutputTokensMillions, scenario)
		if err != nil {
			return err
		}
	}
	return nil
}
