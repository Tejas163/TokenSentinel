package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

const demoAssessmentName = "DemoCorp"

type seedDemoResponse struct {
	AssessmentID  int    `json:"assessment_id"`
	ReportURL     string `json:"report_url"`
	EntriesAdded  int    `json:"entries_added"`
	AlreadyExists bool   `json:"already_exists"`
}

func handleAdminSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Find or create assessment
	var aid int
	err := db.QueryRow(`SELECT id FROM assessments WHERE company_name = $1 LIMIT 1`, demoAssessmentName).Scan(&aid)
	if err != nil {
		// Create assessment
		assessmentJSON := `{
			"company_name": "DemoCorp",
			"cloud_vendor": "aws",
			"gpu_configs": [
				{"type": "A100", "count": 4, "region": "us-east-1", "hourly_price": 3.50, "reserved": true},
				{"type": "H100", "count": 2, "region": "us-west-2", "hourly_price": 4.50, "reserved": false}
			],
			"monthly_request_volume": 1000000,
			"token_distribution": {"input_pct": 0.75, "output_pct": 0.25},
			"current_monthly_spend": 12000,
			"providers_used": [
				{"name": "openai", "models": ["gpt-4o", "gpt-4o-mini", "gpt-4-turbo"], "monthly_spend": 7000},
				{"name": "anthropic", "models": ["claude-3-opus", "claude-3-sonnet"], "monthly_spend": 3000},
				{"name": "self-hosted", "models": ["llama-3-70b", "mixtral-8x7b"], "monthly_spend": 2000}
			],
			"team_composition": {"developers": 20, "platform_engineers": 3, "devops": 2, "management": 2},
			"source": "manual"
		}`

		var a Assessment
		if err := json.Unmarshal([]byte(assessmentJSON), &a); err != nil {
			http.Error(w, fmt.Sprintf("parse assessment: %v", err), http.StatusInternalServerError)
			return
		}

		err = db.QueryRow(
			`INSERT INTO assessments (company_name, cloud_vendor, gpu_configs, monthly_request_volume,
				token_distribution, current_monthly_spend, providers_used, team_composition, source, version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			RETURNING id`,
			a.CompanyName, a.CloudVendor, toJSON(a.GPUConfigs), a.MonthlyRequestVolume,
			toJSON(a.TokenDistribution), a.CurrentMonthlySpend, toJSON(a.ProvidersUsed),
			toJSON(a.TeamComposition), a.Source, 1,
		).Scan(&aid)
		if err != nil {
			http.Error(w, fmt.Sprintf("insert assessment: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("seed: created assessment id=%d", aid)
	}

	// Run prescriptive engine (re-runs if already exists, clearing stale data)
	report, err := RunAssessment(aid)
	if err != nil {
		log.Printf("seed: run assessment error: %v", err)
	} else {
		log.Printf("seed: engine done — $%.0f current, $%.0f savings, %d recs",
			report.TotalCurrent, report.TotalSavings, len(report.Recommendations))
	}

	// Seed cost entries (past 7 days, 3-5 per hour)
	models := []struct {
		name   string
		team   string
		inMin  int
		inMax  int
		outMin int
		outMax int
	}{
		{"gpt-4o", "engineering", 300, 1200, 100, 400},
		{"gpt-4o-mini", "engineering", 200, 800, 60, 200},
		{"gpt-4-turbo", "product", 400, 1500, 150, 500},
		{"claude-3-opus", "product", 500, 2000, 200, 800},
		{"claude-3-sonnet", "research", 250, 1000, 80, 300},
		{"llama-3-70b", "infra", 300, 900, 100, 350},
		{"mixtral-8x7b", "infra", 150, 600, 50, 200},
	}

	now := time.Now().UTC()
	entries := 0
	for day := 0; day < 7; day++ {
		for hour := 8; hour < 20; hour++ {
			count := 3 + rand.Intn(3)
			for i := 0; i < count; i++ {
				m := models[rand.Intn(len(models))]
				ts := now.Add(-time.Duration(day*24+hour) * time.Hour).Add(-time.Duration(rand.Intn(60)) * time.Minute)
				rid := fmt.Sprintf("seed-%d-%d-%d", day, hour, i)
				_, err := db.Exec(
					`INSERT INTO cost_entries (request_id, model, input_tokens, output_tokens, timestamp, team)
					VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (request_id) DO NOTHING`,
					rid, m.name,
					m.inMin+rand.Intn(m.inMax-m.inMin),
					m.outMin+rand.Intn(m.outMax-m.outMin),
					ts.Format(time.RFC3339), m.team,
				)
				if err == nil {
					entries++
				}
			}
		}
	}
	log.Printf("seed: added %d cost entries", entries)

	// Create monitoring rule
	var ruleID int
	db.QueryRow(`SELECT id FROM monitoring_rules WHERE name = 'Demo: Spend Alert' LIMIT 1`).Scan(&ruleID)
	if ruleID == 0 {
		db.Exec(`INSERT INTO monitoring_rules (name, metric, condition, threshold, cooldown_minutes, enabled, channels)
			VALUES ('Demo: Spend Alert', 'monthly_spend', 'above', 8000, 30, true, '["email"]')`)
	}

	// Create budget rule
	var budgetID int
	db.QueryRow(`SELECT id FROM budget_rules WHERE name = 'Demo: Monthly Budget' LIMIT 1`).Scan(&budgetID)
	if budgetID == 0 {
		db.Exec(`INSERT INTO budget_rules (name, monthly_limit, alert_threshold, enabled)
			VALUES ('Demo: Monthly Budget', 15000, 0.8, true)`)
	}

	encodeJSON(w, seedDemoResponse{
		AssessmentID:  aid,
		ReportURL:     fmt.Sprintf("/api/prescriptive/report/%d", aid),
		EntriesAdded:  entries,
		AlreadyExists: false,
	})
}

func toJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
