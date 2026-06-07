package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/proxyops/internal/engine"
)

func monitorSpendTrends(ctx context.Context) {
	ticker := time.NewTicker(monitoringInterval)
	defer ticker.Stop()
	slog.Info("monitoring: spend trends goroutine started")
	for {
		select {
		case <-ticker.C:
			checkSpendTrends()
		case <-ctx.Done():
			return
		}
	}
}

func trackSavings(ctx context.Context) {
	ticker := time.NewTicker(monitoringInterval)
	defer ticker.Stop()
	slog.Info("monitoring: savings tracking goroutine started")
	for {
		select {
		case <-ticker.C:
			detectSavings()
		case <-ctx.Done():
			return
		}
	}
}

func sendAlerts(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	slog.Info("monitoring: alert dispatch goroutine started")
	for {
		select {
		case <-ticker.C:
			dispatchPendingAlerts()
		case <-ctx.Done():
			return
		}
	}
}

func checkSpendTrends() {
	currentPeriod := "7d"
	previousPeriod := "14d"
	currentSince := recentTime(currentPeriod)
	previousSince := recentTime(previousPeriod)
	previousEnd := currentSince

	currentModels := queryModelCosts(currentSince)
	previousModels := queryModelCostsRange(previousSince, previousEnd)

	for model, currentCost := range currentModels {
		prevCost := previousModels[model]
		if prevCost <= 0 {
			continue
		}
		pctChange := ((currentCost - prevCost) / prevCost) * 100
		absChange := currentCost - prevCost
		if pctChange <= 0 || absChange <= 0 {
			continue
		}
		thresholds := getThresholdsForModel(model)
		if pctChange >= thresholds.PctThreshold && absChange >= thresholds.AbsThreshold {
			createAlert(model, "spend_spike", severityForChange(pctChange),
				fmt.Sprintf("%s spend increased %.0f%% (from $%.0f to $%.0f in 7 days)", model, pctChange, prevCost, currentCost),
				currentCost, prevCost*(1+thresholds.PctThreshold/100))
		}
	}
}

func queryModelCosts(since string) map[string]float64 {
	result := make(map[string]float64)
	for _, mi := range engine.ModelCatalog {
		inputCost := (queryTokenSum("cost_entries", "input_tokens", mi.Name, since) / 1000) * mi.InputPrice
		outputCost := (queryTokenSum("cost_entries", "output_tokens", mi.Name, since) / 1000) * mi.OutputPrice
		total := inputCost + outputCost
		if total > 0 {
			result[mi.Name] += total
		}
	}
	rows, err := db.Query(`SELECT model,
		COALESCE(SUM(input_tokens),0) as total_in,
		COALESCE(SUM(output_tokens),0) as total_out
		FROM cost_entries WHERE timestamp >= $1 AND model NOT IN (
			'gpt-4','gpt-4-turbo','gpt-4o','gpt-4o-mini','gpt-3.5-turbo',
			'claude-3-opus','claude-3-sonnet','claude-3-haiku',
			'gemini-1.5-pro','gemini-1.5-flash',
			'mistral-large','mistral-small',
			'llama-3-70b','llama-3-8b','mixtral-8x7b'
		) GROUP BY model`, since)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var model string
		var totalIn, totalOut float64
		if err := rows.Scan(&model, &totalIn, &totalOut); err != nil {
			slog.Error("scan model costs", "err", err)
			continue
		}
		cost := (totalIn/1000)*30 + (totalOut/1000)*60
		if cost > 0 {
			result[model] += cost
		}
	}
	return result
}

func queryModelCostsRange(since, until string) map[string]float64 {
	result := make(map[string]float64)
	for _, mi := range engine.ModelCatalog {
		inputCost := (queryTokenSumRange("cost_entries", "input_tokens", mi.Name, since, until) / 1000) * mi.InputPrice
		outputCost := (queryTokenSumRange("cost_entries", "output_tokens", mi.Name, since, until) / 1000) * mi.OutputPrice
		total := inputCost + outputCost
		if total > 0 {
			result[mi.Name] += total
		}
	}
	return result
}

func queryTokenSum(table, column, model, since string) float64 {
	var total sql.NullFloat64
	db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(%s),0) FROM %s WHERE model=$1 AND timestamp>=$2`, column, table),
		model, since).Scan(&total)
	return total.Float64
}

func queryTokenSumRange(table, column, model, since, until string) float64 {
	var total sql.NullFloat64
	db.QueryRow(fmt.Sprintf(`SELECT COALESCE(SUM(%s),0) FROM %s WHERE model=$1 AND timestamp>=$2 AND timestamp<$3`, column, table),
		model, since, until).Scan(&total)
	return total.Float64
}

func getThresholdsForModel(model string) MonitoringRule {
	row := db.QueryRow(`SELECT pct_threshold, abs_threshold, period FROM monitoring_rules WHERE (model=$1 OR model='*') AND enabled=true ORDER BY model DESC LIMIT 1`, model)
	var rule MonitoringRule
	err := row.Scan(&rule.PctThreshold, &rule.AbsThreshold, &rule.Period)
	if err == sql.ErrNoRows {
		return getDefaultRule()
	}
	if err != nil {
		return getDefaultRule()
	}
	if rule.PctThreshold <= 0 {
		rule.PctThreshold = 20
	}
	if rule.AbsThreshold <= 0 {
		rule.AbsThreshold = 100
	}
	return rule
}

func severityForChange(pctChange float64) string {
	if pctChange > 100 {
		return "critical"
	}
	if pctChange > 50 {
		return "warning"
	}
	return "info"
}

func createAlert(model, alertType, severity, message string, currentValue, thresholdValue float64) {
	existing := 0
	db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE model=$1 AND alert_type=$2 AND acknowledged_at IS NULL AND dismissed_at IS NULL AND created_at > NOW() - INTERVAL '30 minutes'`,
		model, alertType).Scan(&existing)
	if existing > 0 {
		return
	}
	_, err := db.Exec(`INSERT INTO alerts (model, alert_type, severity, message, current_value, threshold_value) VALUES ($1,$2,$3,$4,$5,$6)`,
		model, alertType, severity, message, currentValue, thresholdValue)
	if err != nil {
		slog.Error("monitoring: failed to create alert", "err", err)
		return
	}
	slog.Info("monitoring: alert created", "type", alertType, "model", model, "severity", severity)
}

func detectSavings() {
	for _, mi := range engine.ModelCatalog {
		recentSince := recentTime("7d")
		priorSince := recentTime("14d")
		recentEnd := recentSince

		recentInput := queryTokenSum("cost_entries", "input_tokens", mi.Name, recentSince)
		recentOutput := queryTokenSum("cost_entries", "output_tokens", mi.Name, recentSince)
		priorInput := queryTokenSumRange("cost_entries", "input_tokens", mi.Name, priorSince, recentEnd)
		priorOutput := queryTokenSumRange("cost_entries", "output_tokens", mi.Name, priorSince, recentEnd)

		recentCost := (recentInput/1000)*mi.InputPrice + (recentOutput/1000)*mi.OutputPrice
		priorCost := (priorInput/1000)*mi.InputPrice + (priorOutput/1000)*mi.OutputPrice

		if priorCost <= 0 || recentCost <= 0 {
			continue
		}

		dropPct := ((priorCost - recentCost) / priorCost) * 100
		if dropPct >= 30 {
			savings := priorCost - recentCost
			if savings < 10 {
				continue
			}
			existing := 0
			db.QueryRow(`SELECT COUNT(*) FROM savings_events WHERE model=$1 AND detected_at > NOW() - INTERVAL '7 days'`, mi.Name).Scan(&existing)
			if existing > 0 {
				continue
			}
			notes := fmt.Sprintf("Cost dropped %.0f%% (from $%.0f to $%.0f)", dropPct, priorCost, recentCost)
			_, err := db.Exec(`INSERT INTO savings_events (model, detection_method, previous_monthly_cost, current_monthly_cost, estimated_monthly_savings, confidence, notes)
				VALUES ($1,'cost_drop',$2,$3,$4,'high',$5)`,
				mi.Name, priorCost, recentCost, savings, notes)
			if err != nil {
				slog.Error("monitoring: failed to create savings event", "err", err)
				continue
			}
			createAlert(mi.Name, "savings_opportunity", "info",
				fmt.Sprintf("Savings detected on %s: $%.0f/mo (%.0f%% drop)", mi.Name, savings, dropPct),
				recentCost, priorCost)
		}
	}
}

func dispatchPendingAlerts() {
	rows, err := db.Query(`SELECT id, model, alert_type, severity, message, current_value, threshold_value, created_at
		FROM alerts WHERE acknowledged_at IS NULL AND dismissed_at IS NULL
		ORDER BY created_at ASC LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var a Alert
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &a.Model, &a.AlertType, &a.Severity, &a.Message, &a.CurrentValue, &a.ThresholdValue, &createdAt); err != nil {
			slog.Error("scan pending alert", "err", err)
			continue
		}
		dispatchAlert(a)
	}
}

func dispatchAlert(a Alert) {
	rule := getThresholdsForModel(a.Model)

	if rule.WebhookURL != "" && strings.HasPrefix(rule.WebhookURL, "http") {
		payload, err := json.Marshal(map[string]interface{}{
			"type":    a.AlertType,
			"severity": a.Severity,
			"model":   a.Model,
			"message": a.Message,
			"current": a.CurrentValue,
			"threshold": a.ThresholdValue,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			slog.Error("alert webhook marshal", "err", err)
		} else {
			resp, err := signAndPost(rule.WebhookURL, payload)
			if err != nil {
				slog.Error("monitoring: webhook dispatch failed", "alert_id", a.ID, "err", err)
			} else {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	}

	if rule.EmailTo != "" && strings.Contains(rule.EmailTo, "@") {
		sendAlertEmail(rule.EmailTo, a)
	}

	payload, err := json.Marshal(map[string]interface{}{
		"type":    "monitoring_alert",
		"alert_type": a.AlertType,
		"severity": a.Severity,
		"model":   a.Model,
		"message": a.Message,
		"current": a.CurrentValue,
		"threshold": a.ThresholdValue,
	})
	if err != nil {
		slog.Error("alert SSE marshal", "err", err)
	} else {
		events.broadcast(sseEvent{Type: "alert", Data: payload})
	}
}

func getTrendData(model, period string) ([]SpendTrendPoint, error) {
	d, err := time.ParseDuration(period)
	if err != nil {
		d = 7 * 24 * time.Hour
	}
	since := time.Now().UTC().Add(-d)

	rows, err := db.Query(`SELECT DATE(timestamp) as day, SUM(input_tokens + output_tokens) as total_tokens,
		SUM(input_tokens) as total_in, SUM(output_tokens) as total_out
		FROM cost_entries WHERE model=$1 AND timestamp>=$2
		GROUP BY DATE(timestamp) ORDER BY day`, model, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []SpendTrendPoint
	for rows.Next() {
		var day time.Time
		var totalTokens, totalIn, totalOut float64
		if err := rows.Scan(&day, &totalTokens, &totalIn, &totalOut); err != nil {
			slog.Error("scan trend data", "err", err)
			continue
		}
		mi := engine.FindModel(model)
		inputPrice := 30.0
		outputPrice := 60.0
		if mi != nil {
			inputPrice = mi.InputPrice
			outputPrice = mi.OutputPrice
		}
		cost := (totalIn/1000)*inputPrice + (totalOut/1000)*outputPrice
		points = append(points, SpendTrendPoint{
			Date: day.Format("2006-01-02"),
			Cost: math.Round(cost*100) / 100,
		})
	}
	return points, nil
}
