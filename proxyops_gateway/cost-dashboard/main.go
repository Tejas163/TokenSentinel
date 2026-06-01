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
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

//go:embed dashboard.html
var dashboardContent embed.FS

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
	rdb         *redis.Client
	db          *sql.DB
	tmpls       *template.Template
	authAPIKey  string
	events      = newSSEBroker()
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

type AnomalyEntry struct {
	RequestID   string  `json:"request_id"`
	Model       string  `json:"model"`
	TotalTokens int     `json:"total_tokens"`
	Mean        float64 `json:"mean"`
	Stddev      float64 `json:"stddev"`
	ZScore      float64 `json:"z_score"`
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
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/cost_dashboard?sslmode=disable"
	}

	var err error
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open postgres: %v", err)
	}
	if err = initDB(); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}

	tmpls = template.Must(template.ParseFS(dashboardContent, "dashboard.html"))
	authAPIKey = os.Getenv("AUTH_API_KEY")

	go subscribeCostEvents(context.Background())
	go dataRetention(context.Background())
	go detectAnomalies(context.Background())
	go checkBudgets(context.Background())
	go syncTeamBudgets(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleDashboardHealth)
	mux.HandleFunc("/api/dashboard/costs", authMiddleware(handleCosts))
	mux.HandleFunc("/api/dashboard/summary", authMiddleware(handleSummary))
	mux.HandleFunc("/api/dashboard/anomalies", authMiddleware(handleAnomalies))
	mux.HandleFunc("/api/dashboard/events", authMiddleware(handleSSE))
	mux.HandleFunc("/api/admin/budget-rules", authMiddleware(handleBudgetRules))
	mux.HandleFunc("/api/admin/teams", authMiddleware(handleTeams))
	mux.HandleFunc("/api/budget/status", handleBudgetStatus)
	mux.HandleFunc("/", authMiddleware(handleDashboard))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}
	log.Printf("Cost dashboard starting on :%s...", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
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
	return err
}

func dataRetention(ctx context.Context) {
	interval := 24 * time.Hour
	maxAge := 90
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			result, err := db.Exec(`DELETE FROM cost_entries WHERE timestamp < NOW() - $1::interval`, fmt.Sprintf("%d days", maxAge))
			if err != nil {
				log.Printf("data retention prune failed: %v", err)
			} else if n, _ := result.RowsAffected(); n > 0 {
				log.Printf("data retention pruned %d entries older than %d days", n, maxAge)
			}
		case <-ctx.Done():
			return
		}
	}
}

func detectAnomalies(ctx context.Context) {
	interval := 5 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			period := "24h"
			since, _ := time.ParseDuration(period)
			sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

			rows, err := db.Query(
				`SELECT ce.request_id, ce.model, ce.input_tokens + ce.output_tokens,
				        ms.mean, ms.stddev,
				        (ce.input_tokens + ce.output_tokens - ms.mean) / NULLIF(ms.stddev, 0)
				 FROM cost_entries ce
				 JOIN (SELECT model, AVG(input_tokens + output_tokens) AS mean,
				              STDDEV_SAMP(input_tokens + output_tokens) AS stddev
				       FROM cost_entries WHERE timestamp >= $1 GROUP BY model) ms
				   ON ce.model = ms.model
				 WHERE ce.timestamp >= $1
				   AND ms.stddev IS NOT NULL
				   AND ce.input_tokens + ce.output_tokens > ms.mean + 3 * ms.stddev
				 ORDER BY 6 DESC`,
				sinceStr,
			)
			if err != nil {
				log.Printf("anomaly query failed: %v", err)
				continue
			}

			for rows.Next() {
				var a AnomalyEntry
				if err := rows.Scan(&a.RequestID, &a.Model, &a.TotalTokens, &a.Mean, &a.Stddev, &a.ZScore); err != nil {
					continue
				}
				data, _ := json.Marshal(a)
				log.Printf("ANOMALY: %s", data)
				rdb.Publish(ctx, "anomaly:events", string(data))
				events.broad <- sseEvent{Type: "anomaly", Data: data}
			}
			rows.Close()
		case <-ctx.Done():
			return
		}
	}
}

func handleAnomalies(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}
	since, err := time.ParseDuration(period)
	if err != nil {
		http.Error(w, "invalid period", http.StatusBadRequest)
		return
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

	rows, err := db.Query(
		`SELECT ce.request_id, ce.model, ce.input_tokens + ce.output_tokens,
		        ms.mean, ms.stddev,
		        (ce.input_tokens + ce.output_tokens - ms.mean) / NULLIF(ms.stddev, 0)
		 FROM cost_entries ce
		 JOIN (SELECT model, AVG(input_tokens + output_tokens) AS mean,
		              STDDEV_SAMP(input_tokens + output_tokens) AS stddev
		       FROM cost_entries WHERE timestamp >= $1 GROUP BY model) ms
		   ON ce.model = ms.model
		 WHERE ce.timestamp >= $1
		   AND ms.stddev IS NOT NULL
		   AND ce.input_tokens + ce.output_tokens > ms.mean + 3 * ms.stddev
		 ORDER BY 6 DESC`,
		sinceStr,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []AnomalyEntry
	for rows.Next() {
		var a AnomalyEntry
		if err := rows.Scan(&a.RequestID, &a.Model, &a.TotalTokens, &a.Mean, &a.Stddev, &a.ZScore); err != nil {
			continue
		}
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
		data, _ := json.Marshal(map[string]interface{}{
			"request_id":    reqID,
			"model":         entry.Model,
			"input_tokens":  entry.InputTokens,
			"output_tokens": entry.OutputTokens,
			"timestamp":     entry.Timestamp,
			"team":          entry.Team,
		})
		events.broad <- sseEvent{Type: "cost", Data: data}
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
		http.Error(w, "invalid period", http.StatusBadRequest)
		return
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

	rows, err := db.Query(
		`SELECT model, SUM(input_tokens + output_tokens) as total_tokens,
		        SUM(input_tokens) as total_input, SUM(output_tokens) as total_output,
		        COUNT(*) as request_count
		 FROM cost_entries WHERE timestamp >= $1 GROUP BY model ORDER BY total_tokens DESC`,
		sinceStr,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []ModelCost
	for rows.Next() {
		var mc ModelCost
		if err := rows.Scan(&mc.Model, &mc.TotalTokens, &mc.TotalInput, &mc.TotalOutput, &mc.RequestCount); err != nil {
			continue
		}
		if mc.RequestCount > 0 {
			mc.AvgInput = float64(mc.TotalInput) / float64(mc.RequestCount)
			mc.AvgOutput = float64(mc.TotalOutput) / float64(mc.RequestCount)
		}
		results = append(results, mc)
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
		http.Error(w, "invalid period", http.StatusBadRequest)
		return
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)

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

	row := db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens),0),
		        COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		        COUNT(DISTINCT model)
		 FROM cost_entries WHERE timestamp >= $1`,
		sinceStr,
	)
	if err := row.Scan(&summary.TotalRequests, &summary.TotalTokens, &summary.TotalInput, &summary.TotalOutput, &summary.UniqueModels); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		if key == "" || key != authAPIKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
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

					payload, _ := json.Marshal(map[string]interface{}{
						"rule_id":      r.ID,
						"model":        r.Model,
						"period":       r.Period,
						"total_tokens": totalTokens.Int64,
						"max_tokens":   r.MaxTokens,
						"exceeded_by":  totalTokens.Int64 - r.MaxTokens,
						"checked_at":   time.Now().UTC().Format(time.RFC3339),
					})
					resp, postErr := webhookClient.Post(r.WebhookURL, "application/json", bytes.NewReader(payload))
					if postErr != nil {
						log.Printf("webhook post failed for rule %d: %v", r.ID, postErr)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					log.Printf("budget alert fired for rule %d -> %s", r.ID, r.WebhookURL)
				}
			}
			rows.Close()
		case <-ctx.Done():
			return
		}
	}
}

func handleBudgetRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rows, err := db.Query(`SELECT id, model, max_tokens, period, webhook_url, enabled FROM budget_rules ORDER BY id`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var rules []BudgetRule
		for rows.Next() {
			var br BudgetRule
			if err := rows.Scan(&br.ID, &br.Model, &br.MaxTokens, &br.Period, &br.WebhookURL, &br.Enabled); err != nil {
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
			`INSERT INTO budget_rules (model, max_tokens, period, webhook_url) VALUES ($1,$2,$3,$4) RETURNING id, enabled`,
			br.Model, br.MaxTokens, br.Period, br.WebhookURL,
		).Scan(&br.ID, &br.Enabled)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		_, err = db.Exec(`DELETE FROM budget_rules WHERE id = $1`, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	switch r.Method {
	case "GET":
		rows, err := db.Query(`SELECT id, name, monthly_token_budget, period FROM teams ORDER BY name`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var teams []Team
		for rows.Next() {
			var t Team
			if err := rows.Scan(&t.ID, &t.Name, &t.MonthlyTokenBudget, &t.Period); err != nil {
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
			`INSERT INTO teams (name, monthly_token_budget, period) VALUES ($1,$2,$3) RETURNING id`,
			t.Name, t.MonthlyTokenBudget, t.Period,
		).Scan(&t.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		key := fmt.Sprintf("budget:team:%s:limit", t.Name)
		rdb.Set(r.Context(), key, t.MonthlyTokenBudget, 0)
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
		err = db.QueryRow(`DELETE FROM teams WHERE id = $1 RETURNING name`, id).Scan(&name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rdb.Del(r.Context(), fmt.Sprintf("budget:team:%s:limit", name))
		rdb.Del(r.Context(), fmt.Sprintf("budget:team:%s:used", name))
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpls.Execute(w, nil)
}


