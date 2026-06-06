package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

//go:embed dashboard.html
var dashboardContent embed.FS

//go:embed enterprise.html
var enterpriseHTML embed.FS

//go:embed static/styles.css
var staticCSS embed.FS

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}

func lookupEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		if fallback == "" {
			log.Fatalf("required env var %q is not set", key)
		}
		log.Printf("env %q not set, using default: %s", key, fallback)
		return fallback
	}
	return v
}

type sseEvent struct {
	Type string
	Data []byte
}

type sseBroker struct {
	subs    map[chan sseEvent]bool
	reg     chan chan sseEvent
	unreg   chan chan sseEvent
	broad   chan sseEvent
}

func newSSEBroker() *sseBroker {
	b := &sseBroker{
		subs:  make(map[chan sseEvent]bool),
		reg:   make(chan chan sseEvent),
		unreg: make(chan chan sseEvent),
		broad: make(chan sseEvent, 64),
	}
	go b.run()
	return b
}

func (b *sseBroker) broadcast(evt sseEvent) {
	select {
	case b.broad <- evt:
	default:
		log.Printf("sse: broker channel full, dropping %s event", evt.Type)
	}
}

func (b *sseBroker) run() {
	for {
		select {
		case ch := <-b.reg:
			b.subs[ch] = true
		case ch := <-b.unreg:
			delete(b.subs, ch)
			close(ch)
		case ev := <-b.broad:
			for ch := range b.subs {
				select {
				case ch <- ev:
				default:
				}
			}
		}
	}
}

func (b *sseBroker) subscribe() chan sseEvent {
	ch := make(chan sseEvent, 16)
	b.reg <- ch
	return ch
}

func (b *sseBroker) unsubscribe(ch chan sseEvent) {
	b.unreg <- ch
}

var (
	rdb            *redis.Client
	db             *sql.DB
	tmpls          *template.Template
	authAPIKey     string
	events         = newSSEBroker()
	appStore       Store
	anomalyZScore  = 3.0

	monitoringInterval = 5 * time.Minute
	anomalyInterval    = 5 * time.Minute
	retentionMaxAge    = 90
)

type CostEntry struct {
	RequestID    string `json:"request_id"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Timestamp    string `json:"timestamp"`
	IngestedAt   string `json:"ingested_at"`
}

type Team struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	MonthlyTokenBudget  int64  `json:"monthly_token_budget"`
	Period              string `json:"period"`
}

type BudgetRule struct {
	ID         int    `json:"id"`
	Model      string `json:"model"`
	MaxTokens  int64  `json:"max_tokens"`
	Period     string `json:"period"`
	WebhookURL string `json:"webhook_url"`
	Enabled    bool   `json:"enabled"`
}

type EscalationPolicy struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	AlertType      string `json:"alert_type"`
	Model          string `json:"model"`
	Severity       string `json:"severity"`
	TimeoutMinutes int    `json:"timeout_minutes"`
	WebhookURL     string `json:"webhook_url"`
	Enabled        bool   `json:"enabled"`
	CreatedAt      string `json:"created_at"`
}

type contextKey string

const orgCtxKey contextKey = "org_id"

func getOrgID(r *http.Request) string {
	if v := r.Context().Value(orgCtxKey); v != nil {
		return v.(string)
	}
	return ""
}

type AnomalyEntry struct {
	ID          int       `json:"id,omitempty"`
	RequestID   string    `json:"request_id"`
	Model       string    `json:"model"`
	TotalTokens int       `json:"total_tokens"`
	Mean        float64   `json:"mean"`
	Stddev      float64   `json:"stddev"`
	ZScore      float64   `json:"z_score"`
	DetectedAt  string    `json:"detected_at,omitempty"`
	OrgID       string    `json:"org_id,omitempty"`
}

type ModelCost struct {
	Model        string  `json:"model"`
	TotalTokens  int     `json:"total_tokens"`
	TotalInput   int     `json:"total_input"`
	TotalOutput  int     `json:"total_output"`
	RequestCount int     `json:"request_count"`
	AvgInput     float64 `json:"avg_input"`
	AvgOutput    float64 `json:"avg_output"`
}

func main() {
	redisAddr := lookupEnv("REDIS_ADDR", "localhost:6379")
	dsn := lookupEnv("DATABASE_URL", "postgres://localhost:5432/cost_dashboard?sslmode=disable")
	port := lookupEnv("PORT", "3001")
	authAPIKey = lookupEnv("AUTH_API_KEY", "")
	if v := lookupEnv("ANOMALY_Z_SCORE", "3.0"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			anomalyZScore = f
		}
	}
	if v := lookupEnv("MONITORING_INTERVAL", "5m"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			monitoringInterval = d
		}
	}
	if v := lookupEnv("ANOMALY_INTERVAL", "5m"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			anomalyInterval = d
		}
	}
	if v := lookupEnv("RETENTION_MAX_DAYS", "90"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			retentionMaxAge = n
		}
	}

	var err error
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open postgres: %v", err)
	}
	if err = initDB(); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	if err = initPrescriptiveTables(db); err != nil {
		log.Fatalf("failed to init prescriptive tables: %v", err)
	}
	if err = initMonitoringTables(db); err != nil {
		log.Fatalf("failed to init monitoring tables: %v", err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	tmpls = template.Must(template.ParseFS(dashboardContent, "dashboard.html"))
	appStore = &pgStore{db: db}
	initEmailConfig()

	syncModelCatalogToRedis(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go subscribeCostEvents(ctx)
	go monitorSpendTrends(ctx)
	go trackSavings(ctx)
	go sendAlerts(ctx)
	go dataRetention(ctx)
	go detectAnomalies(ctx)
	go checkBudgets(ctx)
	go syncTeamBudgets(ctx)
	go costDigest(ctx)
	go refreshCostSummary(ctx)
	go monitorAlertsEscalation(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", authMiddleware(handleDashboardHealth))
	mux.HandleFunc("/api/health/all", authMiddleware(handleHealthAll))
	mux.HandleFunc("/api/dashboard/costs", orgMiddleware(handleCosts))
	mux.HandleFunc("/api/dashboard/summary", orgMiddleware(handleSummary))
	mux.HandleFunc("/api/dashboard/anomalies", orgMiddleware(handleAnomalies))
	mux.HandleFunc("/api/dashboard/events", authMiddleware(handleSSE))
	mux.HandleFunc("/api/admin/budget-rules", rateLimitMiddleware(orgMiddleware(handleBudgetRules)))
	mux.HandleFunc("/api/admin/teams", rateLimitMiddleware(orgMiddleware(handleTeams)))
	mux.HandleFunc("/api/admin/pricing", rateLimitMiddleware(orgMiddleware(handleAdminPricing)))
	mux.HandleFunc("/api/admin/pricing/", rateLimitMiddleware(orgMiddleware(handleAdminPricing)))
	mux.HandleFunc("/api/admin/seed-demo", rateLimitMiddleware(authMiddleware(handleAdminSeed)))
	mux.HandleFunc("/api/admin/escalation-policies", rateLimitMiddleware(orgMiddleware(handleEscalationPolicies)))
	mux.HandleFunc("/api/budget/status", orgMiddleware(handleBudgetStatus))
	mux.HandleFunc("/api/playground/models", authMiddleware(handlePlaygroundModels))
	mux.HandleFunc("/api/playground/send", authMiddleware(handlePlaygroundSend))
	mux.HandleFunc("/api/prescriptive/report/", handleReportFrontend)
	mux.HandleFunc("/api/prescriptive/", orgMiddleware(handlePrescriptiveRouter))
	mux.HandleFunc("/api/monitoring/", orgMiddleware(handleMonitoringRouter))
	mux.HandleFunc("/v1/health", authMiddleware(handleDashboardHealth))
	mux.HandleFunc("/v1/health/all", authMiddleware(handleHealthAll))
	mux.HandleFunc("/v1/dashboard/costs", orgMiddleware(handleCosts))
	mux.HandleFunc("/v1/dashboard/summary", orgMiddleware(handleSummary))
	mux.HandleFunc("/v1/dashboard/anomalies", orgMiddleware(handleAnomalies))
	mux.HandleFunc("/v1/dashboard/events", authMiddleware(handleSSE))
	mux.HandleFunc("/v1/admin/budget-rules", rateLimitMiddleware(orgMiddleware(handleBudgetRules)))
	mux.HandleFunc("/v1/admin/teams", rateLimitMiddleware(orgMiddleware(handleTeams)))
	mux.HandleFunc("/v1/admin/pricing", rateLimitMiddleware(orgMiddleware(handleAdminPricing)))
	mux.HandleFunc("/v1/admin/pricing/", rateLimitMiddleware(orgMiddleware(handleAdminPricing)))
	mux.HandleFunc("/v1/admin/seed-demo", rateLimitMiddleware(authMiddleware(handleAdminSeed)))
	mux.HandleFunc("/v1/admin/escalation-policies", rateLimitMiddleware(orgMiddleware(handleEscalationPolicies)))
	mux.HandleFunc("/v1/budget/status", orgMiddleware(handleBudgetStatus))
	mux.HandleFunc("/v1/playground/models", authMiddleware(handlePlaygroundModels))
	mux.HandleFunc("/v1/playground/send", authMiddleware(handlePlaygroundSend))
	mux.HandleFunc("/v1/prescriptive/report/", handleReportFrontend)
	mux.HandleFunc("/v1/prescriptive/", orgMiddleware(handlePrescriptiveRouter))
	mux.HandleFunc("/v1/monitoring/", orgMiddleware(handleMonitoringRouter))
	mux.HandleFunc("/static/styles.css", handleStaticCSS)
	mux.HandleFunc("/assessments", handleAssessmentFrontend)
	mux.HandleFunc("/dashboard", handleDashboard)
	mux.HandleFunc("/enterprise", handleEnterprisePage)
	mux.HandleFunc("/api/enterprise/inquiry", handleEnterpriseInquiry)
	mux.HandleFunc("/", handleLanding)

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Cost dashboard starting on :%s...", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := <-quit
	log.Printf("received signal %v, shutting down...", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced shutdown: %v", err)
	}
	log.Println("server stopped")
}

func initDB() error {
	stmt := `CREATE TABLE IF NOT EXISTS cost_entries (
		id BIGSERIAL PRIMARY KEY,
		request_id TEXT NOT NULL UNIQUE,
		model TEXT NOT NULL,
		input_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		timestamp TIMESTAMPTZ NOT NULL,
		ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		team TEXT NOT NULL DEFAULT ''
	);`
	_, err := db.Exec(stmt)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_model ON cost_entries(model)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_model_timestamp ON cost_entries(model, timestamp)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_timestamp ON cost_entries(timestamp)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS budget_rules (
		id SERIAL PRIMARY KEY,
		model TEXT NOT NULL,
		max_tokens BIGINT NOT NULL,
		period TEXT NOT NULL DEFAULT '24h',
		webhook_url TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT true
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS teams (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		monthly_token_budget BIGINT NOT NULL DEFAULT 0,
		period TEXT NOT NULL DEFAULT '30d'
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_team ON cost_entries(team)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS anomalies (
		id BIGSERIAL PRIMARY KEY,
		request_id TEXT NOT NULL UNIQUE,
		model TEXT NOT NULL,
		total_tokens INTEGER NOT NULL,
		mean DOUBLE PRECISION NOT NULL,
		stddev DOUBLE PRECISION NOT NULL,
		z_score DOUBLE PRECISION NOT NULL,
		detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_anomalies_model ON anomalies(model)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_anomalies_detected ON anomalies(detected_at DESC)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cost_summary_hourly (
		hour_start TIMESTAMPTZ NOT NULL,
		model TEXT NOT NULL,
		total_tokens BIGINT NOT NULL DEFAULT 0,
		total_input BIGINT NOT NULL DEFAULT 0,
		total_output BIGINT NOT NULL DEFAULT 0,
		request_count INTEGER NOT NULL DEFAULT 0,
		org_id TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (hour_start, model, org_id)
	)`)
	if err != nil {
		return err
	}

	// Multi-tenancy: add orgs table and migrate existing tables.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS orgs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE cost_entries ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE anomalies ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE budget_rules ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE teams ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE monitoring_rules ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE savings_events ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS escalated_at TIMESTAMPTZ`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS escalation_policies (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		alert_type TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '*',
		severity TEXT NOT NULL DEFAULT 'warning',
		timeout_minutes INTEGER NOT NULL DEFAULT 30,
		webhook_url TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		org_id TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

func dataRetention(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			result, err := db.Exec(`DELETE FROM cost_entries WHERE timestamp < NOW() - $1::interval`, fmt.Sprintf("%d days", retentionMaxAge))
			if err != nil {
				log.Printf("data retention cost prune failed: %v", err)
			} else if n, _ := result.RowsAffected(); n > 0 {
				log.Printf("data retention pruned %d cost entries older than %d days", n, retentionMaxAge)
			}
			result, err = db.Exec(`DELETE FROM anomalies WHERE detected_at < NOW() - $1::interval`, fmt.Sprintf("%d days", retentionMaxAge))
			if err != nil {
				log.Printf("data retention anomaly prune failed: %v", err)
			} else if n, _ := result.RowsAffected(); n > 0 {
				log.Printf("data retention pruned %d anomalies older than %d days", n, retentionMaxAge)
			}
		case <-ctx.Done():
			return
		}
	}
}

func queryAnomalies(sinceStr string) ([]AnomalyEntry, error) {
	rows, err := db.Query(
		fmt.Sprintf(`SELECT ce.request_id, ce.model, ce.input_tokens + ce.output_tokens,
		        ms.mean, ms.stddev,
		        (ce.input_tokens + ce.output_tokens - ms.mean) / NULLIF(ms.stddev, 0),
		        ce.org_id
		 FROM cost_entries ce
		 JOIN (SELECT model, AVG(input_tokens + output_tokens) AS mean,
		              STDDEV_SAMP(input_tokens + output_tokens) AS stddev
		       FROM cost_entries WHERE timestamp >= $1 GROUP BY model) ms
		   ON ce.model = ms.model
		 WHERE ce.timestamp >= $1
		   AND ms.stddev IS NOT NULL
		   AND ce.input_tokens + ce.output_tokens > ms.mean + %f * ms.stddev
		 ORDER BY 6 DESC`, anomalyZScore),
		sinceStr,
	)
	if err != nil {
		return nil, fmt.Errorf("anomaly query: %w", err)
	}
	defer rows.Close()

	var results []AnomalyEntry
	for rows.Next() {
		var a AnomalyEntry
		if err := rows.Scan(&a.RequestID, &a.Model, &a.TotalTokens, &a.Mean, &a.Stddev, &a.ZScore, &a.OrgID); err != nil {
			continue
		}
		results = append(results, a)
	}
	return results, nil
}

func refreshCostSummary(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	// Run once on startup for recent data.
	runCostSummaryRefresh()
	for {
		select {
		case <-ticker.C:
			runCostSummaryRefresh()
		case <-ctx.Done():
			return
		}
	}
}

func runCostSummaryRefresh() {
	_, err := db.Exec(`INSERT INTO cost_summary_hourly (hour_start, model, total_tokens, total_input, total_output, request_count, org_id)
		SELECT date_trunc('hour', timestamp) as hour_start, model,
		       SUM(input_tokens + output_tokens), SUM(input_tokens), SUM(output_tokens), COUNT(*), org_id
		FROM cost_entries
		WHERE timestamp < date_trunc('hour', NOW())
		GROUP BY hour_start, model, org_id
		ON CONFLICT (hour_start, model, org_id) DO UPDATE SET
			total_tokens = EXCLUDED.total_tokens,
			total_input = EXCLUDED.total_input,
			total_output = EXCLUDED.total_output,
			request_count = EXCLUDED.request_count`)
	if err != nil {
		log.Printf("refresh cost summary: %v", err)
	}
}

func monitorAlertsEscalation(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			checkAndEscalate(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func checkAndEscalate(ctx context.Context) {
	policies, err := db.Query(`SELECT id, name, alert_type, model, severity, timeout_minutes, webhook_url, org_id
		FROM escalation_policies WHERE enabled = true`)
	if err != nil {
		log.Printf("escalation policies query: %v", err)
		return
	}
	defer policies.Close()

	type policy struct {
		id             int
		name           string
		alertType      string
		model          string
		severity       string
		timeoutMinutes int
		webhookURL     string
		orgID          string
	}
	var matched []policy
	for policies.Next() {
		var p policy
		if err := policies.Scan(&p.id, &p.name, &p.alertType, &p.model, &p.severity, &p.timeoutMinutes, &p.webhookURL, &p.orgID); err != nil {
			log.Printf("scan escalation policy: %v", err)
			continue
		}
		matched = append(matched, p)
	}

	for _, p := range matched {
		orgFilter := ""
		args := []interface{}{}
		argIdx := 1
		if p.orgID != "" {
			orgFilter = fmt.Sprintf(" AND org_id = $%d", argIdx)
			args = append(args, p.orgID)
			argIdx++
		}
		if p.alertType != "" {
			orgFilter += fmt.Sprintf(" AND alert_type = $%d", argIdx)
			args = append(args, p.alertType)
			argIdx++
		}
		if p.model != "" && p.model != "*" {
			orgFilter += fmt.Sprintf(" AND model = $%d", argIdx)
			args = append(args, p.model)
			argIdx++
		}
		if p.severity != "" {
			orgFilter += fmt.Sprintf(" AND severity = $%d", argIdx)
			args = append(args, p.severity)
			argIdx++
		}

		interval := fmt.Sprintf("%d minutes", p.timeoutMinutes)
		query := `SELECT id, model, alert_type, message FROM alerts
			WHERE acknowledged_at IS NULL AND dismissed_at IS NULL AND escalated_at IS NULL
			AND created_at < NOW() - $` + fmt.Sprintf("%d", argIdx) + `::interval` + orgFilter
		args = append(args, interval)

		rows, err := db.Query(query, args...)
		if err != nil {
			log.Printf("escalation alert query (policy=%s): %v", p.name, err)
			continue
		}
		for rows.Next() {
			var alertID int
			var model, alertType, message string
			if err := rows.Scan(&alertID, &model, &alertType, &message); err != nil {
				log.Printf("scan alert for escalation: %v", err)
				continue
			}
			payload, _ := json.Marshal(map[string]interface{}{
				"escalation_policy": p.name,
				"alert_id":          alertID,
				"model":             model,
				"alert_type":        alertType,
				"message":           message,
			})
			resp, err := webhookClient.Post(p.webhookURL, "application/json", bytes.NewReader(payload))
			if err != nil {
				log.Printf("escalation webhook (policy=%s, alert=%d): %v", p.name, alertID, err)
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			db.Exec(`UPDATE alerts SET escalated_at = NOW() WHERE id = $1`, alertID)
			log.Printf("escalation triggered policy=%s alert=%d url=%s", p.name, alertID, p.webhookURL)
		}
		rows.Close()
	}
}

func detectAnomalies(ctx context.Context) {
	ticker := time.NewTicker(anomalyInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sinceStr := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
			entries, err := queryAnomalies(sinceStr)
			if err != nil {
				log.Printf("detect anomalies: %v", err)
				continue
			}
			for _, a := range entries {
				_, err := db.Exec(`INSERT INTO anomalies (request_id, model, total_tokens, mean, stddev, z_score, org_id)
					VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (request_id) DO NOTHING`,
					a.RequestID, a.Model, a.TotalTokens, a.Mean, a.Stddev, a.ZScore, a.OrgID)
				if err != nil {
					log.Printf("persist anomaly: %v", err)
				}
				data, err := json.Marshal(a)
				if err != nil {
					log.Printf("anomaly marshal: %v", err)
					continue
				}
				log.Printf("ANOMALY: %s", data)
				rdb.Publish(ctx, "anomaly:events", string(data))
				events.broadcast(sseEvent{Type: "anomaly", Data: data})
			}
		case <-ctx.Done():
			return
		}
	}
}

func handleAnomalies(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("live") == "true" {
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "24h"
		}
		since, err := time.ParseDuration(period)
		if err != nil {
			http.Error(w, "invalid period: valid values: 1h, 6h, 24h, 72h, 168h", http.StatusBadRequest)
			return
		}
		sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)
		results, err := queryAnomalies(sinceStr)
		if err != nil {
			log.Printf("anomalies handler: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
		return
	}

	period := r.URL.Query().Get("period")
	limit := parseIntParam(r, "limit", 100)
	offset := parseIntParam(r, "offset", 0)
	if period == "" {
		period = "168h"
	}
	since, err := time.ParseDuration(period)
	if err != nil {
		period = "168h"
		since = 168 * time.Hour
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

	orgID := getOrgID(r)

	var rows *sql.Rows
	if orgID != "" {
		rows, err = db.Query(`SELECT id, request_id, model, total_tokens, mean, stddev, z_score, detected_at
			FROM anomalies WHERE detected_at >= $1 AND org_id = $2 ORDER BY detected_at DESC LIMIT $3 OFFSET $4`,
			sinceStr, orgID, limit, offset)
	} else {
		rows, err = db.Query(`SELECT id, request_id, model, total_tokens, mean, stddev, z_score, detected_at
			FROM anomalies WHERE detected_at >= $1 ORDER BY detected_at DESC LIMIT $2 OFFSET $3`, sinceStr, limit, offset)
	}
	if err != nil {
		log.Printf("anomalies history query: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []AnomalyEntry
	for rows.Next() {
		var a AnomalyEntry
		var detectedAt time.Time
		if err := rows.Scan(&a.ID, &a.RequestID, &a.Model, &a.TotalTokens, &a.Mean, &a.Stddev, &a.ZScore, &detectedAt); err != nil {
			log.Printf("scan anomaly: %v", err)
			continue
		}
		a.DetectedAt = detectedAt.Format(time.RFC3339)
		results = append(results, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func subscribeCostEvents(ctx context.Context) {
	pubsub := rdb.Subscribe(ctx, "health:events")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for msg := range ch {
		if strings.HasPrefix(msg.Payload, "cost:") {
			reqID := strings.TrimPrefix(msg.Payload, "cost:")
			ingestCost(ctx, reqID)
		}
	}
}

func ingestCost(ctx context.Context, reqID string) {
	costKey := fmt.Sprintf("sentinel:%s:cost", reqID)
	data, err := rdb.Get(ctx, costKey).Result()
	if err == redis.Nil {
		return
	}
	if err != nil {
		log.Printf("failed to read cost key %s: %v", costKey, err)
		return
	}

	var entry struct {
		Model        string `json:"model"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
		Timestamp    string `json:"timestamp"`
		Team         string `json:"team"`
	}
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		log.Printf("failed to parse cost data for %s: %v", reqID, err)
		return
	}

	if entry.Model == "" || entry.Timestamp == "" {
		return
	}

	result, err := db.Exec(
		`INSERT INTO cost_entries (request_id, model, input_tokens, output_tokens, timestamp, team) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (request_id) DO NOTHING`,
		reqID, entry.Model, entry.InputTokens, entry.OutputTokens, entry.Timestamp, entry.Team,
	)
	if err != nil {
		log.Printf("failed to insert cost entry %s: %v", reqID, err)
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		data, err := json.Marshal(map[string]interface{}{
			"request_id":    reqID,
			"model":         entry.Model,
			"input_tokens":  entry.InputTokens,
			"output_tokens": entry.OutputTokens,
			"timestamp":     entry.Timestamp,
			"team":          entry.Team,
		})
		if err != nil {
			log.Printf("ingest marshal: %v", err)
		} else {
			events.broadcast(sseEvent{Type: "cost", Data: data})
		}
		total := entry.InputTokens + entry.OutputTokens
		if entry.Team != "" {
			usedKey := fmt.Sprintf("budget:team:%s:used", entry.Team)
			rdb.IncrBy(ctx, usedKey, int64(total))
			rdb.Expire(ctx, usedKey, 720*time.Hour)
		}
	}
}

func handleCosts(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "1h"
	}

	since, err := time.ParseDuration(period)
	if err != nil {
		http.Error(w, "invalid period: valid values: 1h, 6h, 24h, 72h, 168h", http.StatusBadRequest)
		return
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

	limit := parseIntParam(r, "limit", 100)
	offset := parseIntParam(r, "offset", 0)
	orgID := getOrgID(r)

	useAggregated := r.URL.Query().Get("aggregated") == "true"

	var results []ModelCost

	if useAggregated {
		var rows *sql.Rows
		if orgID != "" {
			rows, err = db.Query(
				`SELECT model, COALESCE(SUM(total_tokens),0), COALESCE(SUM(total_input),0),
				        COALESCE(SUM(total_output),0), COALESCE(SUM(request_count),0)
				 FROM cost_summary_hourly WHERE hour_start >= $1 AND org_id = $2
				 GROUP BY model ORDER BY SUM(total_tokens) DESC LIMIT $3 OFFSET $4`,
				sinceStr, orgID, limit, offset,
			)
		} else {
			rows, err = db.Query(
				`SELECT model, COALESCE(SUM(total_tokens),0), COALESCE(SUM(total_input),0),
				        COALESCE(SUM(total_output),0), COALESCE(SUM(request_count),0)
				 FROM cost_summary_hourly WHERE hour_start >= $1
				 GROUP BY model ORDER BY SUM(total_tokens) DESC LIMIT $2 OFFSET $3`,
				sinceStr, limit, offset,
			)
		}
		if err != nil {
			log.Printf("costs aggregated query error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mc ModelCost
			if err := rows.Scan(&mc.Model, &mc.TotalTokens, &mc.TotalInput, &mc.TotalOutput, &mc.RequestCount); err != nil {
				log.Printf("scan aggregated cost row: %v", err)
				continue
			}
			if mc.RequestCount > 0 {
				mc.AvgInput = float64(mc.TotalInput) / float64(mc.RequestCount)
				mc.AvgOutput = float64(mc.TotalOutput) / float64(mc.RequestCount)
			}
			results = append(results, mc)
		}
	} else {
		var rows *sql.Rows
		if orgID != "" {
			rows, err = db.Query(
				`SELECT model, SUM(input_tokens + output_tokens) as total_tokens,
				        SUM(input_tokens) as total_input, SUM(output_tokens) as total_output,
				        COUNT(*) as request_count
				 FROM cost_entries WHERE timestamp >= $1 AND org_id = $2
				 GROUP BY model ORDER BY total_tokens DESC LIMIT $3 OFFSET $4`,
				sinceStr, orgID, limit, offset,
			)
		} else {
			rows, err = db.Query(
				`SELECT model, SUM(input_tokens + output_tokens) as total_tokens,
				        SUM(input_tokens) as total_input, SUM(output_tokens) as total_output,
				        COUNT(*) as request_count
				 FROM cost_entries WHERE timestamp >= $1
				 GROUP BY model ORDER BY total_tokens DESC LIMIT $2 OFFSET $3`,
				sinceStr, limit, offset,
			)
		}
		if err != nil {
			log.Printf("costs query error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mc ModelCost
			if err := rows.Scan(&mc.Model, &mc.TotalTokens, &mc.TotalInput, &mc.TotalOutput, &mc.RequestCount); err != nil {
				log.Printf("scan cost row: %v", err)
				continue
			}
			if mc.RequestCount > 0 {
				mc.AvgInput = float64(mc.TotalInput) / float64(mc.RequestCount)
				mc.AvgOutput = float64(mc.TotalOutput) / float64(mc.RequestCount)
			}
			results = append(results, mc)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func handleSummary(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}
	since, err := time.ParseDuration(period)
	if err != nil {
		http.Error(w, "invalid period: valid values: 1h, 6h, 24h, 72h, 168h", http.StatusBadRequest)
		return
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

	orgID := getOrgID(r)

	var summary struct {
		TotalRequests int     `json:"total_requests"`
		TotalTokens   int     `json:"total_tokens"`
		TotalInput    int     `json:"total_input"`
		TotalOutput   int     `json:"total_output"`
		UniqueModels  int     `json:"unique_models"`
		Period        string  `json:"period"`
		AvgTokensPer  float64 `json:"avg_tokens_per_request"`
	}
	summary.Period = period

	var row *sql.Row
	if orgID != "" {
		row = db.QueryRow(
			`SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens),0),
			        COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
			        COUNT(DISTINCT model)
			 FROM cost_entries WHERE timestamp >= $1 AND org_id = $2`,
			sinceStr, orgID,
		)
	} else {
		row = db.QueryRow(
			`SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens),0),
			        COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
			        COUNT(DISTINCT model)
			 FROM cost_entries WHERE timestamp >= $1`,
			sinceStr,
		)
	}
	if err := row.Scan(&summary.TotalRequests, &summary.TotalTokens, &summary.TotalInput, &summary.TotalOutput, &summary.UniqueModels); err != nil {
		log.Printf("summary query error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if summary.TotalRequests > 0 {
		summary.AvgTokensPer = float64(summary.TotalTokens) / float64(summary.TotalRequests)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authAPIKey == "" {
			next(w, r)
			return
		}
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			if b := r.Header.Get("Authorization"); len(b) > 7 && strings.EqualFold(b[:7], "Bearer ") {
				key = b[7:]
			}
		}
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if key == "" || key != authAPIKey {
			log.Printf("auth failure: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func orgMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		orgID := r.Header.Get("X-Org-Id")
		if orgID == "" {
			orgID = r.URL.Query().Get("org_id")
		}
		if orgID != "" {
			ctx := context.WithValue(r.Context(), orgCtxKey, orgID)
			r = r.WithContext(ctx)
		}
		next(w, r)
	})
}

func signPayload(payload []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func signAndPost(url string, payload []byte) (*http.Response, error) {
	sig := signPayload(payload, authAPIKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TokenSentinel-Signature", sig)
	return webhookClient.Do(req)
}

func handleDashboardHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err := rdb.Ping(ctx).Err()
	status := "ok"
	code := http.StatusOK
	if err != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

type serviceHealth struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

type healthAllResponse struct {
	Status   string          `json:"status"`
	Services []serviceHealth `json:"services"`
}

func handleHealthAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var services []serviceHealth

	// Redis health
	start := time.Now()
	if err := rdb.Ping(ctx).Err(); err != nil {
		services = append(services, serviceHealth{Service: "redis", Status: "down", Error: err.Error(), Latency: time.Since(start).String()})
	} else {
		services = append(services, serviceHealth{Service: "redis", Status: "up", Latency: time.Since(start).String()})
	}

	// Postgres health
	start = time.Now()
	if err := db.PingContext(ctx); err != nil {
		services = append(services, serviceHealth{Service: "postgres", Status: "down", Error: err.Error(), Latency: time.Since(start).String()})
	} else {
		services = append(services, serviceHealth{Service: "postgres", Status: "up", Latency: time.Since(start).String()})
	}

	// Go router health
	start = time.Now()
	routerURL := os.Getenv("ROUTER_URL")
	if routerURL == "" {
		routerURL = "http://go-router:8080"
	}
	if resp, err := http.Get(routerURL + "/health"); err != nil {
		services = append(services, serviceHealth{Service: "go-router", Status: "down", Error: err.Error(), Latency: time.Since(start).String()})
	} else {
		resp.Body.Close()
		status := "up"
		if resp.StatusCode != http.StatusOK {
			status = "degraded"
		}
		services = append(services, serviceHealth{Service: "go-router", Status: status, Latency: time.Since(start).String()})
	}

	overall := "healthy"
	for _, s := range services {
		if s.Status == "down" {
			overall = "degraded"
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthAllResponse{Status: overall, Services: services})
}

var webhookClient = &http.Client{Timeout: 10 * time.Second}

var (
	lastNotified   = make(map[int]time.Time)
	lastNotifiedMu sync.Mutex
)

func checkBudgets(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rows, err := db.Query(`SELECT id, model, max_tokens, period, webhook_url FROM budget_rules WHERE enabled = true`)
			if err != nil {
				log.Printf("budget rules query failed: %v", err)
				continue
			}
			for rows.Next() {
				var r BudgetRule
				if err := rows.Scan(&r.ID, &r.Model, &r.MaxTokens, &r.Period, &r.WebhookURL); err != nil {
					log.Printf("scan budget rule row: %v", err)
					continue
				}
				since, parseErr := time.ParseDuration(r.Period)
				if parseErr != nil {
					continue
				}
				sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

				var totalTokens sql.NullInt64
				query := `SELECT COALESCE(SUM(input_tokens + output_tokens), 0) FROM cost_entries WHERE timestamp >= $1`
				if r.Model != "*" {
					query += ` AND model = $2`
				}
				var rowErr error
				if r.Model != "*" {
					rowErr = db.QueryRow(query, sinceStr, r.Model).Scan(&totalTokens)
				} else {
					rowErr = db.QueryRow(query, sinceStr).Scan(&totalTokens)
				}
				if rowErr != nil {
					continue
				}

				if totalTokens.Int64 > r.MaxTokens {
					lastNotifiedMu.Lock()
					last := lastNotified[r.ID]
					notify := time.Since(last) > 30*time.Minute
					if notify {
						lastNotified[r.ID] = time.Now()
					}
					lastNotifiedMu.Unlock()
					if !notify {
						continue
					}

				payload, err := json.Marshal(map[string]interface{}{
						"rule_id":      r.ID,
						"model":        r.Model,
						"period":       r.Period,
						"total_tokens": totalTokens.Int64,
						"max_tokens":   r.MaxTokens,
						"exceeded_by":  totalTokens.Int64 - r.MaxTokens,
						"checked_at":   time.Now().UTC().Format(time.RFC3339),
					})
					if err != nil {
						log.Printf("budget webhook marshal: %v", err)
						continue
					}
					resp, postErr := signAndPost(r.WebhookURL, payload)
					if postErr != nil {
						log.Printf("budget webhook post error: %v", postErr)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					log.Printf("budget alert fired for rule %d -> %s (signed)", r.ID, r.WebhookURL)
				}
			}
			rows.Close()
		case <-ctx.Done():
			return
		}
	}
}

func handleBudgetRules(w http.ResponseWriter, r *http.Request) {
	orgID := getOrgID(r)
	switch r.Method {
	case "GET":
		limit := parseIntParam(r, "limit", 100)
		offset := parseIntParam(r, "offset", 0)
		var rows *sql.Rows
		var err error
		if orgID != "" {
			rows, err = db.Query(`SELECT id, model, max_tokens, period, webhook_url, enabled FROM budget_rules WHERE org_id = $1 ORDER BY id LIMIT $2 OFFSET $3`, orgID, limit, offset)
		} else {
			rows, err = db.Query(`SELECT id, model, max_tokens, period, webhook_url, enabled FROM budget_rules ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
		}
		if err != nil {
			log.Printf("budget rules query error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var rules []BudgetRule
		for rows.Next() {
			var br BudgetRule
			if err := rows.Scan(&br.ID, &br.Model, &br.MaxTokens, &br.Period, &br.WebhookURL, &br.Enabled); err != nil {
				log.Printf("scan budget rules list: %v", err)
				continue
			}
			rules = append(rules, br)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rules)

	case "POST":
		var br BudgetRule
		if err := json.NewDecoder(r.Body).Decode(&br); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if br.Model == "" || br.MaxTokens <= 0 || br.WebhookURL == "" {
			http.Error(w, "model, max_tokens, and webhook_url required", http.StatusBadRequest)
			return
		}
		if br.Period == "" {
			br.Period = "24h"
		}
		err := db.QueryRow(
			`INSERT INTO budget_rules (model, max_tokens, period, webhook_url, org_id) VALUES ($1,$2,$3,$4,$5) RETURNING id, enabled`,
			br.Model, br.MaxTokens, br.Period, br.WebhookURL, orgID,
		).Scan(&br.ID, &br.Enabled)
		if err != nil {
			log.Printf("budget rules insert error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		log.Printf("audit: budget rule created id=%d model=%s from %s", br.ID, br.Model, r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(br)

	case "DELETE":
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if orgID != "" {
			_, err = db.Exec(`DELETE FROM budget_rules WHERE id = $1 AND org_id = $2`, id, orgID)
		} else {
			_, err = db.Exec(`DELETE FROM budget_rules WHERE id = $1`, id)
		}
		if err != nil {
			log.Printf("budget rules delete error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		log.Printf("audit: budget rule deleted id=%d from %s", id, r.RemoteAddr)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func costDigest(ctx context.Context) {
	webhookURL := os.Getenv("DIGEST_WEBHOOK_URL")
	if webhookURL == "" {
		return
	}
	schedule := os.Getenv("DIGEST_SCHEDULE")
	if schedule == "" {
		schedule = "24h"
	}
	interval, err := time.ParseDuration(schedule)
	if err != nil {
		log.Printf("invalid DIGEST_SCHEDULE %q, defaulting to 24h", schedule)
		interval = 24 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sinceStr := time.Now().UTC().Add(-interval).Format(time.RFC3339)

			rows, err := db.Query(
				`SELECT model, SUM(input_tokens + output_tokens), SUM(input_tokens),
				        SUM(output_tokens), COUNT(*)
				 FROM cost_entries WHERE timestamp >= $1 GROUP BY model ORDER BY SUM(input_tokens + output_tokens) DESC`,
				sinceStr,
			)
			if err != nil {
				log.Printf("digest query failed: %v", err)
				continue
			}

			var fields []map[string]interface{}
			var totalTokens int64
			for rows.Next() {
				var model string
				var total, inp, out, count int64
				if err := rows.Scan(&model, &total, &inp, &out, &count); err != nil {
					log.Printf("scan digest row: %v", err)
					continue
				}
				totalTokens += total
				fields = append(fields, map[string]interface{}{
					"name":   fmt.Sprintf("%s (%d req)", model, count),
					"value":  fmt.Sprintf("%dK tokens (in: %dK, out: %dK)", total/1000, inp/1000, out/1000),
					"short":  true,
				})
			}
			rows.Close()

			payload := map[string]interface{}{
				"text": fmt.Sprintf("TokenSentinel Cost Digest (%s)", interval.String()),
				"attachments": []map[string]interface{}{
					{
						"title": "Cost Summary",
						"fields": fields,
						"footer": fmt.Sprintf("Total: %dK tokens", totalTokens/1000),
						"color":  "#38bdf8",
					},
				},
			}
			data, err := json.Marshal(payload)
			if err != nil {
				log.Printf("digest marshal: %v", err)
				continue
			}
			resp, err := signAndPost(webhookURL, data)
			if err != nil {
				log.Printf("digest webhook failed: %v", err)
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			log.Printf("cost digest sent to %s (signed)", webhookURL)
		case <-ctx.Done():
			return
		}
	}
}

func syncTeamBudgets(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rows, err := db.Query(`SELECT name, monthly_token_budget, period FROM teams`)
			if err != nil {
				log.Printf("team budget sync query failed: %v", err)
				continue
			}
			for rows.Next() {
				var t Team
				if err := rows.Scan(&t.Name, &t.MonthlyTokenBudget, &t.Period); err != nil {
					log.Printf("scan team sync row: %v", err)
					continue
				}
				key := fmt.Sprintf("budget:team:%s:limit", t.Name)
				rdb.Set(ctx, key, t.MonthlyTokenBudget, 0)
			}
			rows.Close()
		case <-ctx.Done():
			return
		}
	}
}

func handleTeams(w http.ResponseWriter, r *http.Request) {
	orgID := getOrgID(r)
	switch r.Method {
	case "GET":
		limit := parseIntParam(r, "limit", 100)
		offset := parseIntParam(r, "offset", 0)
		var rows *sql.Rows
		var err error
		if orgID != "" {
			rows, err = db.Query(`SELECT id, name, monthly_token_budget, period FROM teams WHERE org_id = $1 ORDER BY name LIMIT $2 OFFSET $3`, orgID, limit, offset)
		} else {
			rows, err = db.Query(`SELECT id, name, monthly_token_budget, period FROM teams ORDER BY name LIMIT $1 OFFSET $2`, limit, offset)
		}
		if err != nil {
			log.Printf("teams query error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var teams []Team
		for rows.Next() {
			var t Team
			if err := rows.Scan(&t.ID, &t.Name, &t.MonthlyTokenBudget, &t.Period); err != nil {
				log.Printf("scan teams list: %v", err)
				continue
			}
			teams = append(teams, t)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(teams)

	case "POST":
		var t Team
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if t.Name == "" || t.MonthlyTokenBudget <= 0 {
			http.Error(w, "name and monthly_token_budget required", http.StatusBadRequest)
			return
		}
		if t.Period == "" {
			t.Period = "30d"
		}
		err := db.QueryRow(
			`INSERT INTO teams (name, monthly_token_budget, period, org_id) VALUES ($1,$2,$3,$4) RETURNING id`,
			t.Name, t.MonthlyTokenBudget, t.Period, orgID,
		).Scan(&t.ID)
		if err != nil {
			log.Printf("teams insert error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		key := fmt.Sprintf("budget:team:%s:limit", t.Name)
		rdb.Set(r.Context(), key, t.MonthlyTokenBudget, 0)
		log.Printf("audit: team created id=%d name=%s from %s", t.ID, t.Name, r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(t)

	case "DELETE":
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var name string
		if orgID != "" {
			err = db.QueryRow(`DELETE FROM teams WHERE id = $1 AND org_id = $2 RETURNING name`, id, orgID).Scan(&name)
		} else {
			err = db.QueryRow(`DELETE FROM teams WHERE id = $1 RETURNING name`, id).Scan(&name)
		}
		if err != nil {
			log.Printf("teams delete error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rdb.Del(r.Context(), fmt.Sprintf("budget:team:%s:limit", name))
		rdb.Del(r.Context(), fmt.Sprintf("budget:team:%s:used", name))
		log.Printf("audit: team deleted id=%d name=%s from %s", id, name, r.RemoteAddr)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleEscalationPolicies(w http.ResponseWriter, r *http.Request) {
	orgID := getOrgID(r)
	switch r.Method {
	case "GET":
		var rows *sql.Rows
		var err error
		if orgID != "" {
			rows, err = db.Query(`SELECT id, name, alert_type, model, severity, timeout_minutes, webhook_url, enabled, created_at
				FROM escalation_policies WHERE org_id = $1 ORDER BY name`, orgID)
		} else {
			rows, err = db.Query(`SELECT id, name, alert_type, model, severity, timeout_minutes, webhook_url, enabled, created_at
				FROM escalation_policies ORDER BY name`)
		}
		if err != nil {
			log.Printf("escalation policies query error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var policies []EscalationPolicy
		for rows.Next() {
			var p EscalationPolicy
			var createdAt time.Time
			if err := rows.Scan(&p.ID, &p.Name, &p.AlertType, &p.Model, &p.Severity, &p.TimeoutMinutes, &p.WebhookURL, &p.Enabled, &createdAt); err != nil {
				log.Printf("scan escalation policy: %v", err)
				continue
			}
			p.CreatedAt = createdAt.Format(time.RFC3339)
			policies = append(policies, p)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policies)

	case "POST":
		var p EscalationPolicy
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if p.Name == "" || p.WebhookURL == "" {
			http.Error(w, "name and webhook_url required", http.StatusBadRequest)
			return
		}
		if p.TimeoutMinutes <= 0 {
			p.TimeoutMinutes = 30
		}
		if p.Severity == "" {
			p.Severity = "warning"
		}
		if p.Model == "" {
			p.Model = "*"
		}
		err := db.QueryRow(
			`INSERT INTO escalation_policies (name, alert_type, model, severity, timeout_minutes, webhook_url, enabled, org_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			p.Name, p.AlertType, p.Model, p.Severity, p.TimeoutMinutes, p.WebhookURL, p.Enabled, orgID,
		).Scan(&p.ID)
		if err != nil {
			log.Printf("create escalation policy error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		log.Printf("audit: escalation policy created id=%d name=%s from %s", p.ID, p.Name, r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)

	case "DELETE":
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if orgID != "" {
			_, err = db.Exec(`DELETE FROM escalation_policies WHERE id = $1 AND org_id = $2`, id, orgID)
		} else {
			_, err = db.Exec(`DELETE FROM escalation_policies WHERE id = $1`, id)
		}
		if err != nil {
			log.Printf("delete escalation policy error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		log.Printf("audit: escalation policy deleted id=%d from %s", id, r.RemoteAddr)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleBudgetStatus(w http.ResponseWriter, r *http.Request) {
	team := r.URL.Query().Get("team")
	if team == "" {
		http.Error(w, "team required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	limit, err := rdb.Get(ctx, fmt.Sprintf("budget:team:%s:limit", team)).Int64()
	if err == redis.Nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"team": team, "budgeted": false})
		return
	}
	if err != nil {
		log.Printf("budget status query error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	used, _ := rdb.Get(ctx, fmt.Sprintf("budget:team:%s:used", team)).Int64()
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"team":       team,
		"budgeted":   true,
		"limit":      limit,
		"used":       used,
		"remaining":  remaining,
		"over_budget": used >= limit,
	})
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := events.subscribe()
	defer events.unsubscribe(ch)

	for {
		select {
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	css, _ := staticCSS.ReadFile("static/styles.css")
	w.Write(css)
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	data := map[string]string{"APIKey": authAPIKey}
	tmpls.Execute(w, data)
}


