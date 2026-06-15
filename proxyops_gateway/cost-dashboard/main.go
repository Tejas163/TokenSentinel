package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/proxyops/internal/engine"
	"github.com/redis/go-redis/v9"
)

//go:embed dashboard.html
var dashboardContent embed.FS

//go:embed enterprise.html
var enterpriseHTML embed.FS

//go:embed static/styles.css
var staticCSS embed.FS

var (
	rdb            *redis.Client
	db             *sql.DB
	tmpls          *template.Template
	authAPIKey     string
	events         = newSSEBroker()
	appStore       engine.Store
	anomalyZScore  = 3.0
	monitoringInterval = 5 * time.Minute
	anomalyInterval    = 5 * time.Minute
	retentionMaxAge    = 90
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, AddSource: true})))

	shutdownOTel := initOTel()
	defer shutdownOTel()

	redisAddr := lookupEnv("REDIS_ADDR", "localhost:6379")
	dsn := lookupEnv("DATABASE_URL", "postgres://localhost:5432/cost_dashboard?sslmode=disable")
	port := lookupEnv("PORT", "3001")
	authAPIKey = os.Getenv("AUTH_API_KEY")
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
		fatal("failed to open postgres", err)
	}
	if err = initDB(); err != nil {
		fatal("failed to init db", err)
	}
	if err = initPrescriptiveTables(db); err != nil {
		fatal("failed to init prescriptive tables", err)
	}
	if err = initMonitoringTables(db); err != nil {
		fatal("failed to init monitoring tables", err)
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

	replayMissedCostEvents(ctx)
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
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", metricsMiddleware(authMiddleware(handleDashboardHealth)))
	mux.HandleFunc("/api/health/all", metricsMiddleware(authMiddleware(handleHealthAll)))
	mux.HandleFunc("/api/dashboard/costs", metricsMiddleware(orgMiddleware(handleCosts)))
	mux.HandleFunc("/api/dashboard/summary", metricsMiddleware(orgMiddleware(handleSummary)))
	mux.HandleFunc("/api/dashboard/cost-timeseries", metricsMiddleware(orgMiddleware(handleCostTimeSeries)))
	mux.HandleFunc("/api/dashboard/anomalies", metricsMiddleware(orgMiddleware(handleAnomalies)))
	mux.HandleFunc("/api/dashboard/events", metricsMiddleware(authMiddleware(handleSSE)))
	mux.HandleFunc("/api/admin/budget-rules", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleBudgetRules))))
	mux.HandleFunc("/api/admin/teams", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleTeams))))
	mux.HandleFunc("/api/admin/pricing", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleAdminPricing))))
	mux.HandleFunc("/api/admin/pricing/", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleAdminPricing))))
	mux.HandleFunc("/api/admin/seed-demo", metricsMiddleware(rateLimitMiddleware(authMiddleware(handleAdminSeed))))
	mux.HandleFunc("/api/admin/escalation-policies", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleEscalationPolicies))))
	mux.HandleFunc("/api/admin/keys", metricsMiddleware(rateLimitMiddleware(authMiddleware(handleAdminKeys))))
	mux.HandleFunc("/api/budget/status", metricsMiddleware(orgMiddleware(handleBudgetStatus)))
	mux.HandleFunc("/api/playground/models", metricsMiddleware(authMiddleware(handlePlaygroundModels)))
	mux.HandleFunc("/api/playground/send", metricsMiddleware(authMiddleware(handlePlaygroundSend)))
	mux.HandleFunc("/api/prescriptive/report/", metricsMiddleware(handleReportFrontend))
	mux.HandleFunc("/api/prescriptive/", metricsMiddleware(orgMiddleware(handlePrescriptiveRouter)))
	mux.HandleFunc("/api/monitoring/", metricsMiddleware(orgMiddleware(handleMonitoringRouter)))
	mux.HandleFunc("/v1/health", metricsMiddleware(authMiddleware(handleDashboardHealth)))
	mux.HandleFunc("/v1/health/all", metricsMiddleware(authMiddleware(handleHealthAll)))
	mux.HandleFunc("/v1/dashboard/costs", metricsMiddleware(orgMiddleware(handleCosts)))
	mux.HandleFunc("/v1/dashboard/summary", metricsMiddleware(orgMiddleware(handleSummary)))
	mux.HandleFunc("/v1/dashboard/cost-timeseries", metricsMiddleware(orgMiddleware(handleCostTimeSeries)))
	mux.HandleFunc("/v1/dashboard/anomalies", metricsMiddleware(orgMiddleware(handleAnomalies)))
	mux.HandleFunc("/v1/dashboard/events", metricsMiddleware(authMiddleware(handleSSE)))
	mux.HandleFunc("/v1/admin/budget-rules", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleBudgetRules))))
	mux.HandleFunc("/v1/admin/teams", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleTeams))))
	mux.HandleFunc("/v1/admin/pricing", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleAdminPricing))))
	mux.HandleFunc("/v1/admin/pricing/", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleAdminPricing))))
	mux.HandleFunc("/v1/admin/seed-demo", metricsMiddleware(rateLimitMiddleware(authMiddleware(handleAdminSeed))))
	mux.HandleFunc("/v1/admin/escalation-policies", metricsMiddleware(rateLimitMiddleware(orgMiddleware(handleEscalationPolicies))))
	mux.HandleFunc("/v1/admin/keys", metricsMiddleware(rateLimitMiddleware(authMiddleware(handleAdminKeys))))
	mux.HandleFunc("/v1/budget/status", metricsMiddleware(orgMiddleware(handleBudgetStatus)))
	mux.HandleFunc("/v1/playground/models", metricsMiddleware(authMiddleware(handlePlaygroundModels)))
	mux.HandleFunc("/v1/playground/send", metricsMiddleware(authMiddleware(handlePlaygroundSend)))
	mux.HandleFunc("/v1/prescriptive/report/", metricsMiddleware(handleReportFrontend))
	mux.HandleFunc("/v1/prescriptive/", metricsMiddleware(orgMiddleware(handlePrescriptiveRouter)))
	mux.HandleFunc("/v1/monitoring/", metricsMiddleware(orgMiddleware(handleMonitoringRouter)))
	mux.HandleFunc("/login", metricsMiddleware(handleLogin))
	mux.HandleFunc("/logout", metricsMiddleware(handleLogout))
	mux.HandleFunc("/static/styles.css", metricsMiddleware(handleStaticCSS))
	mux.HandleFunc("/assessments", metricsMiddleware(handleAssessmentFrontend))
	mux.HandleFunc("/dashboard", metricsMiddleware(handleDashboard))
	mux.HandleFunc("/enterprise", metricsMiddleware(handleEnterprisePage))
	mux.HandleFunc("/api/enterprise/inquiry", metricsMiddleware(handleEnterpriseInquiry))
	mux.HandleFunc("/", metricsMiddleware(handleLanding))

	srv := &http.Server{Addr: ":" + port, Handler: otelMiddleware(mux.ServeHTTP)}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("server error", err)
		}
	}()

	sig := <-quit
	slog.Info("shutting down", "signal", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fatal("server forced shutdown", err)
	}
	slog.Info("server stopped")
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
			slog.Error("data retention cost prune failed", "err", err)
		} else if n, _ := result.RowsAffected(); n > 0 {
			slog.Info("data retention pruned cost entries", "count", n, "maxAgeDays", retentionMaxAge)
		}
		result, err = db.Exec(`DELETE FROM anomalies WHERE detected_at < NOW() - $1::interval`, fmt.Sprintf("%d days", retentionMaxAge))
		if err != nil {
			slog.Error("data retention anomaly prune failed", "err", err)
		} else if n, _ := result.RowsAffected(); n > 0 {
			slog.Info("data retention pruned anomalies", "count", n, "maxAgeDays", retentionMaxAge)
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
		slog.Error("refresh cost summary", "err", err)
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
		slog.Error("escalation policies query", "err", err)
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
			slog.Error("scan escalation policy", "err", err)
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
			slog.Error("escalation alert query", "policy", p.name, "err", err)
			continue
		}
		for rows.Next() {
			var alertID int
			var model, alertType, message string
			if err := rows.Scan(&alertID, &model, &alertType, &message); err != nil {
				slog.Error("scan alert for escalation", "err", err)
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
				slog.Error("escalation webhook", "policy", p.name, "alertID", alertID, "err", err)
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			db.Exec(`UPDATE alerts SET escalated_at = NOW() WHERE id = $1`, alertID)
			slog.Info("escalation triggered", "policy", p.name, "alertID", alertID, "url", p.webhookURL)
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
				slog.Error("detect anomalies", "err", err)
				continue
			}
			for _, a := range entries {
				_, err := db.Exec(`INSERT INTO anomalies (request_id, model, total_tokens, mean, stddev, z_score, org_id)
					VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (request_id) DO NOTHING`,
					a.RequestID, a.Model, a.TotalTokens, a.Mean, a.Stddev, a.ZScore, a.OrgID)
				if err != nil {
					slog.Error("persist anomaly", "err", err)
				}
				data, err := json.Marshal(a)
				if err != nil {
					slog.Error("anomaly marshal", "err", err)
					continue
				}
				slog.Warn("anomaly detected", "request_id", a.RequestID, "model", a.Model, "z_score", a.ZScore)
				anomaliesDetected.Inc()
				rdb.Publish(ctx, "anomaly:events", string(data))
				events.broadcast(sseEvent{Type: "anomaly", Data: data})
			}
		case <-ctx.Done():
			return
		}
	}
}

func replayMissedCostEvents(ctx context.Context) {
	var cursor uint64
	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, "sentinel:*:cost", 100).Result()
		if err != nil {
			slog.Error("failed to scan cost keys for replay", "err", err)
			return
		}
		for _, key := range keys {
			reqID := strings.TrimPrefix(key, "sentinel:")
			reqID = strings.TrimSuffix(reqID, ":cost")
			if reqID != "" {
				ingestCost(ctx, reqID)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
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
		slog.Error("failed to read cost key", "key", costKey, "err", err)
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
		slog.Error("failed to parse cost data", "request_id", reqID, "err", err)
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
		slog.Error("failed to insert cost entry", "request_id", reqID, "err", err)
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		costEntriesTotal.Inc()
		data, err := json.Marshal(map[string]interface{}{
			"request_id":    reqID,
			"model":         entry.Model,
			"input_tokens":  entry.InputTokens,
			"output_tokens": entry.OutputTokens,
			"timestamp":     entry.Timestamp,
			"team":          entry.Team,
		})
		if err != nil {
			slog.Error("ingest marshal", "err", err)
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

func checkBudgets(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		rows, err := db.Query(`SELECT id, model, max_tokens, period, webhook_url FROM budget_rules WHERE enabled = true`)
		if err != nil {
			slog.Error("budget rules query failed", "err", err)
			continue
		}
		for rows.Next() {
			var r BudgetRule
			if err := rows.Scan(&r.ID, &r.Model, &r.MaxTokens, &r.Period, &r.WebhookURL); err != nil {
				slog.Error("scan budget rule row", "err", err)
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
					slog.Error("budget webhook marshal", "err", err)
					continue
				}
				resp, postErr := signAndPost(r.WebhookURL, payload)
				if postErr != nil {
					slog.Error("budget webhook post error", "err", postErr)
					continue
				}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			slog.Info("budget alert fired", "rule_id", r.ID, "webhook_url", r.WebhookURL)
			budgetAlertsFired.Inc()
			}
			}
			rows.Close()
		case <-ctx.Done():
			return
		}
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
		slog.Warn("invalid DIGEST_SCHEDULE, defaulting to 24h", "value", schedule)
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
				slog.Error("digest query failed", "err", err)
				continue
			}

			var fields []map[string]interface{}
			var totalTokens int64
			for rows.Next() {
				var model string
				var total, inp, out, count int64
				if err := rows.Scan(&model, &total, &inp, &out, &count); err != nil {
					slog.Error("scan digest row", "err", err)
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
				slog.Error("digest marshal", "err", err)
				continue
			}
			resp, err := signAndPost(webhookURL, data)
			if err != nil {
				slog.Error("digest webhook failed", "err", err)
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			slog.Info("cost digest sent", "url", webhookURL)
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
			slog.Error("team budget sync query failed", "err", err)
			continue
		}
		for rows.Next() {
			var t Team
			if err := rows.Scan(&t.Name, &t.MonthlyTokenBudget, &t.Period); err != nil {
				slog.Error("scan team sync row", "err", err)
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
