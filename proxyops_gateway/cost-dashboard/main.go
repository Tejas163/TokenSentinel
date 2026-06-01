package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

//go:embed dashboard.html
var dashboardContent embed.FS

var (
	rdb   *redis.Client
	db    *sql.DB
	tmpls *template.Template
)

type CostEntry struct {
	RequestID    string `json:"request_id"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Timestamp    string `json:"timestamp"`
	IngestedAt   string `json:"ingested_at"`
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
	rdb = redis.NewClient(&redis.Options{Addr: redisAddr})

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

	go subscribeCostEvents(context.Background())
	go dataRetention(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleDashboardHealth)
	mux.HandleFunc("/api/dashboard/costs", handleCosts)
	mux.HandleFunc("/api/dashboard/summary", handleSummary)
	mux.HandleFunc("/", handleDashboard)

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
		ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
	}
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		log.Printf("failed to parse cost data for %s: %v", reqID, err)
		return
	}

	if entry.Model == "" || entry.Timestamp == "" {
		return
	}

	_, err = db.Exec(
		`INSERT INTO cost_entries (request_id, model, input_tokens, output_tokens, timestamp) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (request_id) DO NOTHING`,
		reqID, entry.Model, entry.InputTokens, entry.OutputTokens, entry.Timestamp,
	)
	if err != nil {
		log.Printf("failed to insert cost entry %s: %v", reqID, err)
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

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpls.Execute(w, nil)
}


