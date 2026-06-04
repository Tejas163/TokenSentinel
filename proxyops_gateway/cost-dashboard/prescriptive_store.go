package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Store interface {
	GetAssessment(id int) (*Assessment, error)
	QueryLiveCostData(since time.Time) (*AssessmentLiveData, error)
	ReplaceCostProjections(assessmentID int, projections []CostProjection) error
	ReplaceRecommendations(assessmentID int, recs []Recommendation) error
	GetCostProjections(assessmentID int, scenario string) ([]CostProjection, error)
	GetRecommendations(assessmentID int) ([]Recommendation, error)
	InsertCostProjections(assessmentID int, projections []CostProjection, scenario string) error
}

type pgStore struct {
	db *sql.DB
}

func (s *pgStore) GetAssessment(id int) (*Assessment, error) {
	var a Assessment
	var gpuJSON, tokenJSON, providerJSON, teamJSON []byte
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`SELECT id, company_name, cloud_vendor, gpu_configs, monthly_request_volume,
		token_distribution, current_monthly_spend, providers_used, team_composition, source, version, created_at, updated_at
		FROM assessments WHERE id = $1`, id).Scan(
		&a.ID, &a.CompanyName, &a.CloudVendor, &gpuJSON, &a.MonthlyRequestVolume,
		&tokenJSON, &a.CurrentMonthlySpend, &providerJSON, &teamJSON, &a.Source, &a.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("assessment %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query assessment %d: %w", id, err)
	}

	json.Unmarshal(gpuJSON, &a.GPUConfigs)
	json.Unmarshal(tokenJSON, &a.TokenDistribution)
	json.Unmarshal(providerJSON, &a.ProvidersUsed)
	json.Unmarshal(teamJSON, &a.TeamComposition)
	a.CreatedAt = createdAt.Format(time.RFC3339)
	a.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &a, nil
}

func (s *pgStore) QueryLiveCostData(since time.Time) (*AssessmentLiveData, error) {
	sinceStr := since.Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT model, SUM(input_tokens), SUM(output_tokens), COUNT(*)
		FROM cost_entries WHERE timestamp >= $1 GROUP BY model`, sinceStr)
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

func (s *pgStore) ReplaceCostProjections(assessmentID int, projections []CostProjection) error {
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

func (s *pgStore) ReplaceRecommendations(assessmentID int, recs []Recommendation) error {
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

func (s *pgStore) GetCostProjections(assessmentID int, scenario string) ([]CostProjection, error) {
	rows, err := s.db.Query(`SELECT model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions
		FROM cost_projections WHERE assessment_id = $1 AND scenario = $2`, assessmentID, scenario)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projections []CostProjection
	for rows.Next() {
		var cp CostProjection
		if err := rows.Scan(&cp.Model, &cp.Provider, &cp.CurrentMonthlyCost, &cp.ProjectedMonthlyCost, &cp.InputTokensMillions, &cp.OutputTokensMillions); err != nil {
			continue
		}
		cp.Scenario = scenario
		cp.AssessmentID = assessmentID
		projections = append(projections, cp)
	}
	return projections, nil
}

func (s *pgStore) GetRecommendations(assessmentID int) ([]Recommendation, error) {
	rows, err := s.db.Query(`SELECT id, category, description, current_cost, projected_cost, monthly_savings, payback_period_days, priority, created_at
		FROM recommendations WHERE assessment_id = $1 ORDER BY
		CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, monthly_savings DESC`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		var createdAt time.Time
		if err := rows.Scan(&r.ID, &r.Category, &r.Description, &r.CurrentCost, &r.ProjectedCost, &r.MonthlySavings, &r.PaybackPeriodDays, &r.Priority, &createdAt); err != nil {
			continue
		}
		r.AssessmentID = assessmentID
		r.CreatedAt = createdAt.Format(time.RFC3339)
		recs = append(recs, r)
	}
	return recs, nil
}

func (s *pgStore) InsertCostProjections(assessmentID int, projections []CostProjection, scenario string) error {
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


