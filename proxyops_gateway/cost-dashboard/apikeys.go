package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
)

const apiKeyPrefix = "apikey:"

type dashboardKey struct {
	Name               string   `json:"name"`
	Team               string   `json:"team"`
	Status             string   `json:"status"`
	BudgetMonthlyCents int64    `json:"budget_monthly_cents"`
	RateLimitRPS       int64    `json:"rate_limit_rps"`
	AllowedModels      []string `json:"allowed_models"`
	AllowedServices    []string `json:"allowed_services"`
}

func validateDashboardKey(ctx context.Context, key string) (*dashboardKey, error) {
	val, err := rdb.HGetAll(ctx, apiKeyPrefix+key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis error: %w", err)
	}
	if len(val) == 0 {
		return nil, nil
	}
	ak := &dashboardKey{
		Name:               val["name"],
		Team:               val["team"],
		Status:             val["status"],
		BudgetMonthlyCents: parseCents(val["budget_monthly_cents"]),
		RateLimitRPS:       parseRPS(val["rate_limit_rps"]),
	}
	if ak.Status == "" {
		ak.Status = "active"
	}
	if err := json.Unmarshal([]byte(val["allowed_models"]), &ak.AllowedModels); err != nil && val["allowed_models"] != "" {
		slog.Warn("apikey: invalid allowed_models json", "key_prefix", truncate(key, 8))
		ak.AllowedModels = []string{"*"}
	}
	if err := json.Unmarshal([]byte(val["allowed_services"]), &ak.AllowedServices); err != nil && val["allowed_services"] != "" {
		slog.Warn("apikey: invalid allowed_services json", "key_prefix", truncate(key, 8))
		ak.AllowedServices = []string{"*"}
	}
	if ak.AllowedModels == nil {
		ak.AllowedModels = []string{"*"}
	}
	if ak.AllowedServices == nil {
		ak.AllowedServices = []string{"*"}
	}
	return ak, nil
}

func parseCents(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseRPS(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func requestAPIKey(r *http.Request) string {
	if authAPIKey != "" {
		return authAPIKey
	}
	if key := extractAuthKey(r); key != "" {
		return key
	}
	if c, err := r.Cookie("dashboard_api_key"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
