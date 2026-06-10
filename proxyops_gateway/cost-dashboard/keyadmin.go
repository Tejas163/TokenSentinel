package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type keyCreateRequest struct {
	Name               string   `json:"name"`
	Team               string   `json:"team"`
	BudgetMonthlyCents int64    `json:"budget_monthly_cents"`
	RateLimitRPS       int      `json:"rate_limit_rps"`
	AllowedModels      []string `json:"allowed_models"`
	AllowedServices    []string `json:"allowed_services"`
}

type keyUpdateRequest struct {
	Name               *string  `json:"name"`
	Team               *string  `json:"team"`
	Status             *string  `json:"status"`
	BudgetMonthlyCents *int64   `json:"budget_monthly_cents"`
	RateLimitRPS       *int     `json:"rate_limit_rps"`
	AllowedModels      []string `json:"allowed_models"`
	AllowedServices    []string `json:"allowed_services"`
}

type keyResponse struct {
	Key                string   `json:"key"`
	Name               string   `json:"name"`
	Team               string   `json:"team"`
	Status             string   `json:"status"`
	BudgetMonthlyCents int64    `json:"budget_monthly_cents"`
	RateLimitRPS       int      `json:"rate_limit_rps"`
	AllowedModels      []string `json:"allowed_models"`
	AllowedServices    []string `json:"allowed_services"`
	CreatedAt          string   `json:"created_at"`
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return "sk_" + hex.EncodeToString(b), nil
}

func handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var req keyCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Team == "" {
		http.Error(w, "team is required", http.StatusBadRequest)
		return
	}

	key, err := generateAPIKey()
	if err != nil {
		slog.Error("failed to generate key", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if req.AllowedModels == nil {
		req.AllowedModels = []string{"*"}
	}
	if req.AllowedServices == nil {
		req.AllowedServices = []string{"*"}
	}

	hash := map[string]interface{}{
		"name":                 req.Name,
		"team":                 req.Team,
		"status":               "active",
		"budget_monthly_cents": req.BudgetMonthlyCents,
		"rate_limit_rps":       req.RateLimitRPS,
		"allowed_models":       toJSONArr(req.AllowedModels),
		"allowed_services":     toJSONArr(req.AllowedServices),
		"created_at":           time.Now().UTC().Format(time.RFC3339),
	}

	if err := rdb.HSet(context.Background(), apiKeyPrefix+key, hash).Err(); err != nil {
		slog.Error("failed to store key", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := keyResponse{
		Key:                key,
		Name:               req.Name,
		Team:               req.Team,
		Status:             "active",
		BudgetMonthlyCents: req.BudgetMonthlyCents,
		RateLimitRPS:       req.RateLimitRPS,
		AllowedModels:      req.AllowedModels,
		AllowedServices:    req.AllowedServices,
		CreatedAt:          hash["created_at"].(string),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := rdb.Keys(context.Background(), apiKeyPrefix+"*").Result()
	if err != nil {
		slog.Error("failed to list keys", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var resp []keyResponse
	for _, k := range keys {
		fields, err := rdb.HGetAll(context.Background(), k).Result()
		if err != nil || len(fields) == 0 {
			continue
		}
		keyName := strings.TrimPrefix(k, apiKeyPrefix)
		entry := keyResponse{
			Key:                keyName,
			Name:               fields["name"],
			Team:               fields["team"],
			Status:             fields["status"],
			BudgetMonthlyCents: parseCents(fields["budget_monthly_cents"]),
			RateLimitRPS:       int(parseCents(fields["rate_limit_rps"])),
			CreatedAt:          fields["created_at"],
		}
		if entry.Status == "" {
			entry.Status = "active"
		}
		resp = append(resp, entry)
	}

	if resp == nil {
		resp = []keyResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleGetKey(w http.ResponseWriter, r *http.Request) {
	keyName := r.URL.Query().Get("key")
	if keyName == "" {
		http.Error(w, "key query param is required", http.StatusBadRequest)
		return
	}

	fields, err := rdb.HGetAll(context.Background(), apiKeyPrefix+keyName).Result()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(fields) == 0 {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	status := fields["status"]
	if status == "" {
		status = "active"
	}

	resp := keyResponse{
		Key:                keyName,
		Name:               fields["name"],
		Team:               fields["team"],
		Status:             status,
		BudgetMonthlyCents: parseCents(fields["budget_monthly_cents"]),
		RateLimitRPS:       int(parseCents(fields["rate_limit_rps"])),
		CreatedAt:          fields["created_at"],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	keyName := r.URL.Query().Get("key")
	if keyName == "" {
		http.Error(w, "key query param is required", http.StatusBadRequest)
		return
	}

	exists, err := rdb.Exists(context.Background(), apiKeyPrefix+keyName).Result()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if exists == 0 {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	var req keyUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	pipe := rdb.Pipeline()
	ctx := context.Background()
	if req.Name != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "name", *req.Name)
	}
	if req.Team != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "team", *req.Team)
	}
	if req.Status != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "status", *req.Status)
	}
	if req.BudgetMonthlyCents != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "budget_monthly_cents", fmt.Sprintf("%d", *req.BudgetMonthlyCents))
	}
	if req.RateLimitRPS != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "rate_limit_rps", fmt.Sprintf("%d", *req.RateLimitRPS))
	}
	if req.AllowedModels != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "allowed_models", toJSONArr(req.AllowedModels))
	}
	if req.AllowedServices != nil {
		pipe.HSet(ctx, apiKeyPrefix+keyName, "allowed_services", toJSONArr(req.AllowedServices))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("failed to update key", "key_prefix", truncate(keyName, 8), "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	keyName := r.URL.Query().Get("key")
	if keyName == "" {
		http.Error(w, "key query param is required", http.StatusBadRequest)
		return
	}

	result, err := rdb.Del(context.Background(), apiKeyPrefix+keyName).Result()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if result == 0 {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func handleAdminKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleCreateKey(w, r)
	case http.MethodGet:
		if r.URL.Query().Has("key") {
			handleGetKey(w, r)
		} else {
			handleListKeys(w, r)
		}
	case http.MethodPut:
		handleUpdateKey(w, r)
	case http.MethodDelete:
		handleDeleteKey(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func toJSONArr(v []string) string {
	b, _ := json.Marshal(v)
	return string(b)
}
