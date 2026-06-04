package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func handleMonitoringRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/monitoring/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch parts[0] {
	case "rules":
		if len(parts) == 1 {
			handleMonitoringRules(w, r)
		} else if len(parts) == 2 {
			handleMonitoringRules(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	case "alerts":
		if len(parts) == 1 {
			handleAlerts(w, r)
		} else if len(parts) == 3 && parts[2] == "acknowledge" {
			acknowledgeAlert(w, r)
		} else if len(parts) == 3 && parts[2] == "dismiss" {
			dismissAlert(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	case "savings":
		handleSavings(w, r)
	case "trends":
		if len(parts) >= 2 {
			handleTrends(w, r, parts[1])
		} else {
			http.Error(w, "model required", http.StatusBadRequest)
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func handleMonitoringRules(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/monitoring/rules"), "/")
	idStr := ""
	if len(parts) >= 1 && parts[0] != "" {
		idStr = parts[0]
	}
	if len(parts) >= 2 && parts[1] != "" {
		idStr = parts[1]
	}

	if idStr == "" {
		switch r.Method {
		case "GET":
			listMonitoringRules(w, r)
		case "POST":
			createMonitoringRule(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		getMonitoringRule(w, r, id)
	case "PUT":
		updateMonitoringRule(w, r, id)
	case "DELETE":
		deleteMonitoringRule(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listMonitoringRules(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 1000)
	offset := parseIntParam(r, "offset", 0)
	orgID := getOrgID(r)
	var rows *sql.Rows
	var err error
	if orgID != "" {
		rows, err = db.Query(`SELECT id, model, pct_threshold, abs_threshold, period, enabled, webhook_url, email_to, created_at, updated_at
			FROM monitoring_rules WHERE org_id = $1 ORDER BY model LIMIT $2 OFFSET $3`, orgID, limit, offset)
	} else {
		rows, err = db.Query(`SELECT id, model, pct_threshold, abs_threshold, period, enabled, webhook_url, email_to, created_at, updated_at
			FROM monitoring_rules ORDER BY model LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		log.Printf("list monitoring rules error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rules []MonitoringRule
	for rows.Next() {
		var rule MonitoringRule
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&rule.ID, &rule.Model, &rule.PctThreshold, &rule.AbsThreshold, &rule.Period, &rule.Enabled, &rule.WebhookURL, &rule.EmailTo, &createdAt, &updatedAt); err != nil {
			log.Printf("scan monitoring rule: %v", err)
			continue
		}
		rule.CreatedAt = createdAt.Format(time.RFC3339)
		rule.UpdatedAt = updatedAt.Format(time.RFC3339)
		rules = append(rules, rule)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func createMonitoringRule(w http.ResponseWriter, r *http.Request) {
	var rule MonitoringRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if rule.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	if rule.Period == "" {
		rule.Period = "7d"
	}
	if rule.PctThreshold <= 0 {
		rule.PctThreshold = 20
	}
	if rule.AbsThreshold <= 0 {
		rule.AbsThreshold = 100
	}

	orgID := getOrgID(r)

	var id int
	err := db.QueryRow(`INSERT INTO monitoring_rules (model, pct_threshold, abs_threshold, period, enabled, webhook_url, email_to, org_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		rule.Model, rule.PctThreshold, rule.AbsThreshold, rule.Period, rule.Enabled, rule.WebhookURL, rule.EmailTo, orgID).Scan(&id)
	if err != nil {
		log.Printf("create monitoring rule error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rule.ID = id
	rule.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	rule.UpdatedAt = rule.CreatedAt

	log.Printf("audit: monitoring rule created id=%d model=%s from %s", id, rule.Model, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

func getMonitoringRule(w http.ResponseWriter, r *http.Request, id int) {
	var rule MonitoringRule
	var createdAt, updatedAt time.Time
	orgID := getOrgID(r)
	var err error
	if orgID != "" {
		err = db.QueryRow(`SELECT id, model, pct_threshold, abs_threshold, period, enabled, webhook_url, email_to, created_at, updated_at
			FROM monitoring_rules WHERE id = $1 AND org_id = $2`, id, orgID).Scan(
			&rule.ID, &rule.Model, &rule.PctThreshold, &rule.AbsThreshold, &rule.Period, &rule.Enabled, &rule.WebhookURL, &rule.EmailTo, &createdAt, &updatedAt)
	} else {
		err = db.QueryRow(`SELECT id, model, pct_threshold, abs_threshold, period, enabled, webhook_url, email_to, created_at, updated_at
			FROM monitoring_rules WHERE id = $1`, id).Scan(
			&rule.ID, &rule.Model, &rule.PctThreshold, &rule.AbsThreshold, &rule.Period, &rule.Enabled, &rule.WebhookURL, &rule.EmailTo, &createdAt, &updatedAt)
	}
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get monitoring rule error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rule.CreatedAt = createdAt.Format(time.RFC3339)
	rule.UpdatedAt = updatedAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

func updateMonitoringRule(w http.ResponseWriter, r *http.Request, id int) {
	var rule MonitoringRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if rule.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	orgID := getOrgID(r)
	var err error
	if orgID != "" {
		_, err = db.Exec(`UPDATE monitoring_rules SET model=$1, pct_threshold=$2, abs_threshold=$3, period=$4, enabled=$5, webhook_url=$6, email_to=$7, updated_at=NOW() WHERE id=$8 AND org_id=$9`,
			rule.Model, rule.PctThreshold, rule.AbsThreshold, rule.Period, rule.Enabled, rule.WebhookURL, rule.EmailTo, id, orgID)
	} else {
		_, err = db.Exec(`UPDATE monitoring_rules SET model=$1, pct_threshold=$2, abs_threshold=$3, period=$4, enabled=$5, webhook_url=$6, email_to=$7, updated_at=NOW() WHERE id=$8`,
			rule.Model, rule.PctThreshold, rule.AbsThreshold, rule.Period, rule.Enabled, rule.WebhookURL, rule.EmailTo, id)
	}
	if err != nil {
		log.Printf("update monitoring rule error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rule.ID = id
	log.Printf("audit: monitoring rule updated id=%d from %s", id, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

func deleteMonitoringRule(w http.ResponseWriter, r *http.Request, id int) {
	orgID := getOrgID(r)
	var result sql.Result
	var err error
	if orgID != "" {
		result, err = db.Exec(`DELETE FROM monitoring_rules WHERE id = $1 AND org_id = $2`, id, orgID)
	} else {
		result, err = db.Exec(`DELETE FROM monitoring_rules WHERE id = $1`, id)
	}
	if err != nil {
		log.Printf("delete monitoring rule error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	log.Printf("audit: monitoring rule deleted id=%d from %s", id, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `SELECT id, monitoring_rule_id, model, alert_type, severity, message, current_value, threshold_value, acknowledged_at, dismissed_at, created_at
		FROM alerts`
	var conditions []string
	var args []interface{}
	argIdx := 1

	orgID := getOrgID(r)
	if orgID != "" {
		conditions = append(conditions, fmt.Sprintf("org_id = $%d", argIdx))
		args = append(args, orgID)
		argIdx++
	}
	if r.URL.Query().Get("unacknowledged") == "true" {
		conditions = append(conditions, "acknowledged_at IS NULL AND dismissed_at IS NULL")
	}
	if model := r.URL.Query().Get("model"); model != "" {
		conditions = append(conditions, fmt.Sprintf("model = $%d", argIdx))
		args = append(args, model)
		argIdx++
	}
	if alertType := r.URL.Query().Get("type"); alertType != "" {
		conditions = append(conditions, fmt.Sprintf("alert_type = $%d", argIdx))
		args = append(args, alertType)
		argIdx++
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("list alerts error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var monitoringRuleID sql.NullInt64
		var acknowledgedAt, dismissedAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &monitoringRuleID, &a.Model, &a.AlertType, &a.Severity, &a.Message, &a.CurrentValue, &a.ThresholdValue, &acknowledgedAt, &dismissedAt, &createdAt); err != nil {
			log.Printf("scan alert: %v", err)
			continue
		}
		if monitoringRuleID.Valid {
			id := int(monitoringRuleID.Int64)
			a.MonitoringRuleID = &id
		}
		if acknowledgedAt.Valid {
			s := acknowledgedAt.Time.Format(time.RFC3339)
			a.AcknowledgedAt = &s
		}
		if dismissedAt.Valid {
			s := dismissedAt.Time.Format(time.RFC3339)
			a.DismissedAt = &s
		}
		a.CreatedAt = createdAt.Format(time.RFC3339)
		alerts = append(alerts, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func acknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/monitoring/alerts/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	_, err = db.Exec(`UPDATE alerts SET acknowledged_at = NOW() WHERE id = $1 AND acknowledged_at IS NULL`, id)
	if err != nil {
		log.Printf("acknowledge alert error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
}

func dismissAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/monitoring/alerts/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	_, err = db.Exec(`UPDATE alerts SET dismissed_at = NOW() WHERE id = $1 AND dismissed_at IS NULL`, id)
	if err != nil {
		log.Printf("dismiss alert error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "dismissed"})
}

func handleSavings(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`SELECT id, assessment_id, recommendation_id, model, detected_at, detection_method,
		previous_monthly_cost, current_monthly_cost, estimated_monthly_savings, confidence, notes, created_at
		FROM savings_events ORDER BY detected_at DESC LIMIT 100`)
	if err != nil {
		log.Printf("list savings error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []SavingsEvent
	for rows.Next() {
		var e SavingsEvent
		var assessmentID, recommendationID sql.NullInt64
		var detectedAt, createdAt time.Time
		if err := rows.Scan(&e.ID, &assessmentID, &recommendationID, &e.Model, &detectedAt, &e.DetectionMethod,
			&e.PreviousMonthlyCost, &e.CurrentMonthlyCost, &e.EstimatedMonthlySavings, &e.Confidence, &e.Notes, &createdAt); err != nil {
			log.Printf("scan savings event: %v", err)
			continue
		}
		if assessmentID.Valid {
			id := int(assessmentID.Int64)
			e.AssessmentID = &id
		}
		if recommendationID.Valid {
			id := int(recommendationID.Int64)
			e.RecommendationID = &id
		}
		e.DetectedAt = detectedAt.Format(time.RFC3339)
		e.CreatedAt = createdAt.Format(time.RFC3339)
		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func handleTrends(w http.ResponseWriter, r *http.Request, model string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}
	points, err := getTrendData(model, period)
	if err != nil {
		log.Printf("trend data error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []SpendTrendPoint{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}
