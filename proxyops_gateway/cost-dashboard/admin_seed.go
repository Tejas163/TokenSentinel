package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/proxyops/internal/engine"
)

const demoAssessmentName = "DemoCorp"

var (
	seedLimiterMu sync.Mutex
	seedLimits    = make(map[string]time.Time)
)

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

	seedLimiterMu.Lock()
	last, ok := seedLimits[r.RemoteAddr]
	now := time.Now()
	if ok && now.Sub(last) < 30*time.Second {
		seedLimiterMu.Unlock()
		http.Error(w, "rate limit: try again in 30 seconds", http.StatusTooManyRequests)
		return
	}
	seedLimits[r.RemoteAddr] = now
	seedLimiterMu.Unlock()

	// Find or create assessment
	var aid int
	alreadyExists := true
	err := db.QueryRow(`SELECT id FROM assessments WHERE company_name = $1 LIMIT 1`, demoAssessmentName).Scan(&aid)
	if err != nil {
		alreadyExists = false
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

		var a engine.Assessment
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
	report, err := engine.RunAssessment(appStore, aid)
	if err != nil {
		log.Printf("seed: run assessment error: %v", err)
	} else {
		log.Printf("seed: engine done — $%.0f current, $%.0f savings, %d recs",
			report.TotalCurrent, report.TotalSavings, len(report.Recommendations))
	}

	// Seed cost entries (past 7 days, 3-5 per hour)
	entries := seedCostEntries(db)
	log.Printf("seed: added %d cost entries", entries)

	// Create monitoring rule
	var ruleID int
	err = db.QueryRow(`SELECT id FROM monitoring_rules WHERE model = $1 AND pct_threshold = 20 AND abs_threshold = 100 LIMIT 1`, "*").Scan(&ruleID)
	if err != nil {
		ruleID = 0
	}
	if ruleID == 0 {
		_, err := db.Exec(`INSERT INTO monitoring_rules (model, pct_threshold, abs_threshold, period, enabled)
			VALUES ('*', 20, 100, '7d', true)`)
		if err != nil {
			log.Printf("seed: create monitoring rule error: %v", err)
		} else {
			log.Printf("seed: created monitoring rule")
		}
	}

	// Create budget rule
	var budgetID int
	err = db.QueryRow(`SELECT id FROM budget_rules WHERE model = $1 AND max_tokens = 15000000 LIMIT 1`, "*").Scan(&budgetID)
	if err != nil {
		budgetID = 0
	}
	if budgetID == 0 {
		_, err := db.Exec(`INSERT INTO budget_rules (model, max_tokens, period, enabled)
			VALUES ('*', 15000000, '30d', true)`)
		if err != nil {
			log.Printf("seed: create budget rule error: %v", err)
		} else {
			log.Printf("seed: created budget rule")
		}
	}

	encodeJSON(w, seedDemoResponse{
		AssessmentID:  aid,
		ReportURL:     fmt.Sprintf("/api/prescriptive/report/%d", aid),
		EntriesAdded:  entries,
		AlreadyExists: alreadyExists,
	})
}

func toJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

type modelSeed struct {
	name   string
	team   string
	inMin  int
	inMax  int
	outMin int
	outMax int
}

var seedModels = []modelSeed{
	{"gpt-4o", "engineering", 300, 1200, 100, 400},
	{"gpt-4o-mini", "engineering", 200, 800, 60, 200},
	{"gpt-4-turbo", "product", 400, 1500, 150, 500},
	{"claude-3-opus", "product", 500, 2000, 200, 800},
	{"claude-3-sonnet", "research", 250, 1000, 80, 300},
	{"llama-3-70b", "infra", 300, 900, 100, 350},
	{"mixtral-8x7b", "infra", 150, 600, 50, 200},
}

var seedRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

type seedCostRow struct {
	rid       string
	model     string
	inTokens  int
	outTokens int
	ts        string
	team      string
}

func generateSeedRows() []seedCostRow {
	now := time.Now().UTC()
	var rows []seedCostRow
	for day := 0; day < 7; day++ {
		for hour := 8; hour < 20; hour++ {
			count := 3 + seedRNG.Intn(3)
			for i := 0; i < count; i++ {
				m := seedModels[seedRNG.Intn(len(seedModels))]
				ts := now.Add(-time.Duration(day*24+hour) * time.Hour).Add(-time.Duration(seedRNG.Intn(60)) * time.Minute)
				rows = append(rows, seedCostRow{
					rid:       fmt.Sprintf("seed-%d-%d-%d", day, hour, i),
					model:     m.name,
					inTokens:  m.inMin + seedRNG.Intn(m.inMax-m.inMin),
					outTokens: m.outMin + seedRNG.Intn(m.outMax-m.outMin),
					ts:        ts.Format(time.RFC3339),
					team:      m.team,
				})
			}
		}
	}
	return rows
}

func seedCostEntries(db *sql.DB) int {
	rows := generateSeedRows()
	if len(rows) == 0 {
		return 0
	}

	var buf strings.Builder
	buf.WriteString(`INSERT INTO cost_entries (request_id, model, input_tokens, output_tokens, timestamp, team) VALUES `)
	args := make([]interface{}, 0, len(rows)*6)
	argIdx := 1
	for i, r := range rows {
		if i > 0 {
			buf.WriteString(", ")
		}
		fmt.Fprintf(&buf, "($%d,$%d,$%d,$%d,$%d,$%d)", argIdx, argIdx+1, argIdx+2, argIdx+3, argIdx+4, argIdx+5)
		args = append(args, r.rid, r.model, r.inTokens, r.outTokens, r.ts, r.team)
		argIdx += 6
	}
	buf.WriteString(" ON CONFLICT (request_id) DO NOTHING")

	_, err := db.Exec(buf.String(), args...)
	if err != nil {
		log.Printf("seed batch insert error: %v", err)
	}
	return len(rows)
}
