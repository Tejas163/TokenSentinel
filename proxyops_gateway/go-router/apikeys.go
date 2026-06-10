package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const apiKeyPrefix = "apikey:"

type APIKey struct {
	Name               string   `json:"name"`
	Team               string   `json:"team"`
	Status             string   `json:"status"`
	BudgetMonthlyCents int64    `json:"budget_monthly_cents"`
	RateLimitRPS       int      `json:"rate_limit_rps"`
	AllowedModels      []string `json:"allowed_models"`
	AllowedServices    []string `json:"allowed_services"`
	CreatedAt          string   `json:"created_at"`
}

func validateAPIKey(ctx context.Context, key string) (*APIKey, error) {
	val, err := rdb.HGetAll(ctx, apiKeyPrefix+key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis error: %w", err)
	}
	if len(val) == 0 {
		return nil, nil
	}
	ak := &APIKey{
		Name:               val["name"],
		Team:               val["team"],
		Status:             val["status"],
		BudgetMonthlyCents: parseInt64(val["budget_monthly_cents"]),
		RateLimitRPS:       parseInt(val["rate_limit_rps"]),
		CreatedAt:          val["created_at"],
	}
	if ak.Status == "" {
		ak.Status = "active"
	}
	if err := json.Unmarshal([]byte(val["allowed_models"]), &ak.AllowedModels); err != nil && val["allowed_models"] != "" {
		slog.Warn("apikey: invalid allowed_models json", "key_prefix", key[:min(8, len(key))])
		ak.AllowedModels = []string{"*"}
	}
	if err := json.Unmarshal([]byte(val["allowed_services"]), &ak.AllowedServices); err != nil && val["allowed_services"] != "" {
		slog.Warn("apikey: invalid allowed_services json", "key_prefix", key[:min(8, len(key))])
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

func createAPIKey(ctx context.Context, key string, ak *APIKey) error {
	hash := map[string]interface{}{
		"name":                ak.Name,
		"team":                ak.Team,
		"status":              ak.Status,
		"budget_monthly_cents": ak.BudgetMonthlyCents,
		"rate_limit_rps":      ak.RateLimitRPS,
		"allowed_models":      toJSON(ak.AllowedModels),
		"allowed_services":    toJSON(ak.AllowedServices),
		"created_at":          time.Now().UTC().Format(time.RFC3339),
	}
	return rdb.HSet(ctx, apiKeyKey(key), hash).Err()
}

func apiKeyKey(key string) string {
	return apiKeyPrefix + key
}

func parseInt64(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func parseInt(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
