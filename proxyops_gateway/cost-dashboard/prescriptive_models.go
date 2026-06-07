package main

import (
	"database/sql"
	"fmt"
	"log"
)

func initPrescriptiveTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS assessments (
			id SERIAL PRIMARY KEY,
			org_id TEXT NOT NULL DEFAULT '',
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
		`ALTER TABLE assessments ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE assessments ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'USD'`,
		`ALTER TABLE assessments ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(10,4) NOT NULL DEFAULT 1.0`,
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
		`CREATE TABLE IF NOT EXISTS marketplace_templates (
			id SERIAL PRIMARY KEY,
			org_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT 'general',
			template_data JSONB NOT NULL DEFAULT '{}',
			tags TEXT[] NOT NULL DEFAULT '{}',
			download_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recommendations_assessment ON recommendations(assessment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_projections_assessment ON cost_projections(assessment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_assessment_versions_assessment ON assessment_versions(assessment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_marketplace_templates_org ON marketplace_templates(org_id)`,
		`CREATE INDEX IF NOT EXISTS idx_marketplace_templates_category ON marketplace_templates(category)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("prescriptive table init: %w", err)
		}
	}
	log.Println("prescriptive tables initialized")
	return nil
}
