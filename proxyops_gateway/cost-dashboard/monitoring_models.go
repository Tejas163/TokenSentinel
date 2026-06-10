package main

import (
	"database/sql"
	"fmt"
	"time"
)

type MonitoringRule struct {
	ID            int     `json:"id"`
	Model         string  `json:"model"`
	PctThreshold  float64 `json:"pct_threshold"`
	AbsThreshold  float64 `json:"abs_threshold"`
	Period        string  `json:"period"`
	Enabled       bool    `json:"enabled"`
	WebhookURL    string  `json:"webhook_url,omitempty"`
	EmailTo       string  `json:"email_to,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type Alert struct {
	ID               int     `json:"id"`
	MonitoringRuleID *int    `json:"monitoring_rule_id,omitempty"`
	Model            string  `json:"model"`
	AlertType        string  `json:"alert_type"`
	Severity         string  `json:"severity"`
	Message          string  `json:"message"`
	CurrentValue     float64 `json:"current_value"`
	ThresholdValue   float64 `json:"threshold_value"`
	AcknowledgedAt   *string `json:"acknowledged_at,omitempty"`
	DismissedAt      *string `json:"dismissed_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

type SavingsEvent struct {
	ID                   int     `json:"id"`
	AssessmentID         *int    `json:"assessment_id,omitempty"`
	RecommendationID     *int    `json:"recommendation_id,omitempty"`
	Model                string  `json:"model"`
	DetectedAt           string  `json:"detected_at"`
	DetectionMethod      string  `json:"detection_method"`
	PreviousMonthlyCost  float64 `json:"previous_monthly_cost"`
	CurrentMonthlyCost   float64 `json:"current_monthly_cost"`
	EstimatedMonthlySavings float64 `json:"estimated_monthly_savings"`
	Confidence           string  `json:"confidence"`
	Notes                string  `json:"notes,omitempty"`
	CreatedAt            string  `json:"created_at"`
}

type SpendTrendPoint struct {
	Date  string  `json:"date"`
	Cost  float64 `json:"cost"`
}

func initMonitoringTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS monitoring_rules (
			id SERIAL PRIMARY KEY,
			model TEXT NOT NULL DEFAULT '*',
			pct_threshold NUMERIC(5,2) NOT NULL DEFAULT 20,
			abs_threshold NUMERIC(10,2) NOT NULL DEFAULT 100,
			period TEXT NOT NULL DEFAULT '7d',
			enabled BOOLEAN NOT NULL DEFAULT true,
			webhook_url TEXT NOT NULL DEFAULT '',
			email_to TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id SERIAL PRIMARY KEY,
			monitoring_rule_id INTEGER REFERENCES monitoring_rules(id),
			model TEXT NOT NULL,
			alert_type TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT 'warning',
			message TEXT NOT NULL,
			current_value NUMERIC(12,2) NOT NULL DEFAULT 0,
			threshold_value NUMERIC(12,2) NOT NULL DEFAULT 0,
			acknowledged_at TIMESTAMPTZ,
			dismissed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS savings_events (
			id SERIAL PRIMARY KEY,
			assessment_id INTEGER REFERENCES assessments(id),
			recommendation_id INTEGER REFERENCES recommendations(id),
			model TEXT NOT NULL,
			detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			detection_method TEXT NOT NULL DEFAULT 'cost_drop',
			previous_monthly_cost NUMERIC(12,2) NOT NULL DEFAULT 0,
			current_monthly_cost NUMERIC(12,2) NOT NULL DEFAULT 0,
			estimated_monthly_savings NUMERIC(12,2) NOT NULL DEFAULT 0,
			confidence TEXT NOT NULL DEFAULT 'medium',
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_model ON alerts(model)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_unack ON alerts(acknowledged_at, dismissed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_savings_model ON savings_events(model)`,
		`CREATE INDEX IF NOT EXISTS idx_savings_assessment ON savings_events(assessment_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("monitoring table init: %w", err)
		}
	}

	migrations := []string{
		`ALTER TABLE monitoring_rules ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE savings_events ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("monitoring migration: %w", err)
		}
	}

	return nil
}

func getDefaultRule() MonitoringRule {
	return MonitoringRule{
		Model:        "*",
		PctThreshold: 20,
		AbsThreshold: 100,
		Period:       "7d",
		Enabled:      true,
	}
}

func recentTime(duration string) string {
	d, err := time.ParseDuration(duration)
	if err != nil {
		d = 7 * 24 * time.Hour
	}
	return time.Now().UTC().Add(-d).Format(time.RFC3339)
}
