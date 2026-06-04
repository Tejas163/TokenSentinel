package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
)

type GPUConfig struct {
	Type        string  `json:"type"`
	Count       int     `json:"count"`
	Region      string  `json:"region"`
	HourlyPrice float64 `json:"hourly_price"`
	Reserved    bool    `json:"reserved"`
}

type TokenDistribution struct {
	InputPct  float64 `json:"input_pct"`
	OutputPct float64 `json:"output_pct"`
}

type ProviderUsage struct {
	Name         string   `json:"name"`
	Models       []string `json:"models"`
	MonthlySpend float64  `json:"monthly_spend"`
}

type TeamComposition struct {
	Developers        int `json:"developers"`
	PlatformEngineers int `json:"platform_engineers"`
	DevOps            int `json:"devops"`
	Management        int `json:"management"`
}

type Assessment struct {
	ID                   int              `json:"id"`
	CompanyName          string           `json:"company_name"`
	CloudVendor          string           `json:"cloud_vendor"`
	GPUConfigs           []GPUConfig      `json:"gpu_configs"`
	MonthlyRequestVolume int64            `json:"monthly_request_volume"`
	TokenDistribution    TokenDistribution `json:"token_distribution"`
	CurrentMonthlySpend  float64          `json:"current_monthly_spend"`
	ProvidersUsed        []ProviderUsage   `json:"providers_used"`
	TeamComposition      TeamComposition   `json:"team_composition"`
	Source               string           `json:"source"`
	Version              int              `json:"version"`
	CreatedAt            string           `json:"created_at"`
	UpdatedAt            string           `json:"updated_at"`
}

type Recommendation struct {
	ID                int     `json:"id"`
	AssessmentID      int     `json:"assessment_id"`
	Category          string  `json:"category"`
	Description       string  `json:"description"`
	CurrentCost       float64 `json:"current_cost"`
	ProjectedCost     float64 `json:"projected_cost"`
	MonthlySavings    float64 `json:"monthly_savings"`
	PaybackPeriodDays int     `json:"payback_period_days"`
	Priority          string  `json:"priority"`
	CreatedAt         string  `json:"created_at"`
}

type CostProjection struct {
	ID                   int     `json:"id"`
	AssessmentID         int     `json:"assessment_id"`
	Model                string  `json:"model"`
	Provider             string  `json:"provider"`
	CurrentMonthlyCost   float64 `json:"current_monthly_cost"`
	ProjectedMonthlyCost float64 `json:"projected_monthly_cost"`
	InputTokensMillions  float64 `json:"input_tokens_millions"`
	OutputTokensMillions float64 `json:"output_tokens_millions"`
	Scenario             string  `json:"scenario"`
	CreatedAt            string  `json:"created_at"`
}

type AssessmentVersion struct {
	ID            int             `json:"id"`
	AssessmentID  int             `json:"assessment_id"`
	VersionNumber int             `json:"version_number"`
	Snapshot      json.RawMessage `json:"snapshot"`
	CreatedAt     string          `json:"created_at"`
}

type AssessmentReport struct {
	Assessment     Assessment       `json:"assessment"`
	CostBreakdown  []CostProjection `json:"cost_breakdown"`
	Recommendations []Recommendation `json:"recommendations"`
	TotalCurrent   float64          `json:"total_current_spend"`
	TotalProjected float64          `json:"total_projected_spend"`
	TotalSavings   float64          `json:"total_monthly_savings"`
}

func initPrescriptiveTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS assessments (
			id SERIAL PRIMARY KEY,
			company_name TEXT NOT NULL,
			cloud_vendor TEXT NOT NULL DEFAULT '',
			gpu_configs JSONB DEFAULT '[]',
			monthly_request_volume BIGINT NOT NULL DEFAULT 0,
			token_distribution JSONB DEFAULT '{"input_pct": 0.7, "output_pct": 0.3}',
			current_monthly_spend NUMERIC(12,2) NOT NULL DEFAULT 0,
			providers_used JSONB DEFAULT '[]',
			team_composition JSONB DEFAULT '{"developers": 0, "platform_engineers": 0, "devops": 0, "management": 0}',
			source TEXT NOT NULL DEFAULT 'manual',
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS recommendations (
			id SERIAL PRIMARY KEY,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			category TEXT NOT NULL,
			description TEXT NOT NULL,
			current_cost NUMERIC(12,2) NOT NULL DEFAULT 0,
			projected_cost NUMERIC(12,2) NOT NULL DEFAULT 0,
			monthly_savings NUMERIC(12,2) NOT NULL DEFAULT 0,
			payback_period_days INTEGER NOT NULL DEFAULT 0,
			priority TEXT NOT NULL DEFAULT 'medium',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS cost_projections (
			id SERIAL PRIMARY KEY,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			model TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			current_monthly_cost NUMERIC(12,2) NOT NULL DEFAULT 0,
			projected_monthly_cost NUMERIC(12,2) NOT NULL DEFAULT 0,
			input_tokens_millions NUMERIC(12,4) NOT NULL DEFAULT 0,
			output_tokens_millions NUMERIC(12,4) NOT NULL DEFAULT 0,
			scenario TEXT NOT NULL DEFAULT 'base',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS assessment_versions (
			id SERIAL PRIMARY KEY,
			assessment_id INTEGER NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
			version_number INTEGER NOT NULL,
			snapshot JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recommendations_assessment ON recommendations(assessment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_projections_assessment ON cost_projections(assessment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_assessment_versions_assessment ON assessment_versions(assessment_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("prescriptive table init: %w", err)
		}
	}
	log.Println("prescriptive tables initialized")
	return nil
}
