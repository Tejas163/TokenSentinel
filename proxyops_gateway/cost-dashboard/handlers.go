package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

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

func handleHealthAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var services []serviceHealth

	start := time.Now()
	if err := rdb.Ping(ctx).Err(); err != nil {
		services = append(services, serviceHealth{Service: "redis", Status: "down", Error: err.Error(), Latency: time.Since(start).String()})
	} else {
		services = append(services, serviceHealth{Service: "redis", Status: "up", Latency: time.Since(start).String()})
	}

	start = time.Now()
	if err := db.PingContext(ctx); err != nil {
		services = append(services, serviceHealth{Service: "postgres", Status: "down", Error: err.Error(), Latency: time.Since(start).String()})
	} else {
		services = append(services, serviceHealth{Service: "postgres", Status: "up", Latency: time.Since(start).String()})
	}

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
			slog.Error("costs aggregated query error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mc ModelCost
			if err := rows.Scan(&mc.Model, &mc.TotalTokens, &mc.TotalInput, &mc.TotalOutput, &mc.RequestCount); err != nil {
				slog.Error("scan aggregated cost row", "err", err)
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
			slog.Error("costs query error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mc ModelCost
			if err := rows.Scan(&mc.Model, &mc.TotalTokens, &mc.TotalInput, &mc.TotalOutput, &mc.RequestCount); err != nil {
				slog.Error("scan cost row", "err", err)
				continue
			}
			if mc.RequestCount > 0 {
				mc.AvgInput = float64(mc.TotalInput) / float64(mc.RequestCount)
				mc.AvgOutput = float64(mc.TotalOutput) / float64(mc.RequestCount)
			}
			results = append(results, mc)
		}
	}

	for i := range results {
		results[i].Currency = "USD"
		results[i].CurrencySymbol = "$"
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
		TotalRequests  int     `json:"total_requests"`
		TotalTokens    int     `json:"total_tokens"`
		TotalInput     int     `json:"total_input"`
		TotalOutput    int     `json:"total_output"`
		UniqueModels   int     `json:"unique_models"`
		Period         string  `json:"period"`
		AvgTokensPer   float64 `json:"avg_tokens_per_request"`
		Currency       string  `json:"currency"`
		CurrencySymbol string  `json:"currency_symbol"`
	}
	summary.Currency = "USD"
	summary.CurrencySymbol = "$"
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
		slog.Error("summary query error", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if summary.TotalRequests > 0 {
		summary.AvgTokensPer = float64(summary.TotalTokens) / float64(summary.TotalRequests)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func handleCostTimeSeries(w http.ResponseWriter, r *http.Request) {
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
	orgID := getOrgID(r)

	query := `SELECT hour_start, model, total_tokens, total_input, total_output
		FROM cost_summary_hourly WHERE hour_start >= $1`
	args := []interface{}{sinceStr}
	if orgID != "" {
		query += ` AND org_id = $2`
		args = append(args, orgID)
	}
	query += ` ORDER BY hour_start ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		slog.Error("cost timeseries query error", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var points []costTimePoint
	for rows.Next() {
		var hour time.Time
		var model string
		var totalTokens, totalInput, totalOutput int64
		if err := rows.Scan(&hour, &model, &totalTokens, &totalInput, &totalOutput); err != nil {
			slog.Error("scan cost timeseries row", "err", err)
			continue
		}
		price := modelPrice(model)
		cost := (float64(totalInput)/1_000_000)*price.Input + (float64(totalOutput)/1_000_000)*price.Output
		points = append(points, costTimePoint{
			Hour:           hour.Format(time.RFC3339),
			Model:          model,
			Cost:           math.Round(cost*100) / 100,
			Tokens:         totalTokens,
			Currency:       "USD",
			CurrencySymbol: "$",
		})
	}

	if points == nil {
		points = []costTimePoint{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
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
			slog.Error("anomalies handler", "err", err)
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
		slog.Error("anomalies history query", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []AnomalyEntry
	for rows.Next() {
		var a AnomalyEntry
		var detectedAt time.Time
		if err := rows.Scan(&a.ID, &a.RequestID, &a.Model, &a.TotalTokens, &a.Mean, &a.Stddev, &a.ZScore, &detectedAt); err != nil {
			slog.Error("scan anomaly", "err", err)
			continue
		}
		a.DetectedAt = detectedAt.Format(time.RFC3339)
		results = append(results, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
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

	activeConnections.Inc()
	defer activeConnections.Dec()

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
			slog.Error("budget rules query error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var rules []BudgetRule
		for rows.Next() {
			var br BudgetRule
			if err := rows.Scan(&br.ID, &br.Model, &br.MaxTokens, &br.Period, &br.WebhookURL, &br.Enabled); err != nil {
				slog.Error("scan budget rules list", "err", err)
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
			slog.Error("budget rules insert error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("audit: budget rule created", "id", br.ID, "model", br.Model, "remote", r.RemoteAddr)
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
			slog.Error("budget rules delete error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("audit: budget rule deleted", "id", id, "remote", r.RemoteAddr)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
			slog.Error("teams query error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var teams []Team
		for rows.Next() {
			var t Team
			if err := rows.Scan(&t.ID, &t.Name, &t.MonthlyTokenBudget, &t.Period); err != nil {
				slog.Error("scan teams list", "err", err)
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
			slog.Error("teams insert error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		key := fmt.Sprintf("budget:team:%s:limit", t.Name)
		rdb.Set(r.Context(), key, t.MonthlyTokenBudget, 0)
		slog.Info("audit: team created", "id", t.ID, "name", t.Name, "remote", r.RemoteAddr)
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
			slog.Error("teams delete error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rdb.Del(r.Context(), fmt.Sprintf("budget:team:%s:limit", name))
		rdb.Del(r.Context(), fmt.Sprintf("budget:team:%s:used", name))
		slog.Info("audit: team deleted", "id", id, "name", name, "remote", r.RemoteAddr)
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
			slog.Error("escalation policies query error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var policies []EscalationPolicy
		for rows.Next() {
			var p EscalationPolicy
			var createdAt time.Time
			if err := rows.Scan(&p.ID, &p.Name, &p.AlertType, &p.Model, &p.Severity, &p.TimeoutMinutes, &p.WebhookURL, &p.Enabled, &createdAt); err != nil {
				slog.Error("scan escalation policy", "err", err)
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
			slog.Error("create escalation policy error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		slog.Info("audit: escalation policy created", "id", p.ID, "name", p.Name, "remote", r.RemoteAddr)
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
			slog.Error("delete escalation policy error", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("audit: escalation policy deleted", "id", id, "remote", r.RemoteAddr)
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
		slog.Error("budget status query error", "err", err)
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

func handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	css, _ := staticCSS.ReadFile("static/styles.css")
	w.Write(css)
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	apiKey := requestAPIKey(r)
	if apiKey == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	data := map[string]string{"APIKey": apiKey}
	tmpls.Execute(w, data)
}
