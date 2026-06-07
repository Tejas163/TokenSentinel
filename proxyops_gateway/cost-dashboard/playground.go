package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type playgroundSendRequest struct {
	Model       string `json:"model"`
	InputTokens int    `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	Team        string `json:"team"`
}

type playgroundSendResponse struct {
	RequestID    string `json:"request_id"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Team         string `json:"team"`
	Message      string `json:"message"`
}

var playgroundModels = []struct {
	Name   string `json:"name"`
	InputCostPer1K float64 `json:"input_cost_per_1k"`
	OutputCostPer1K float64 `json:"output_cost_per_1k"`
}{
	{"gpt-4o", 0.0025, 0.0100},
	{"gpt-4o-mini", 0.00015, 0.00060},
	{"gpt-4-turbo", 0.0100, 0.0300},
	{"claude-3-opus", 0.0150, 0.0750},
	{"claude-3-sonnet", 0.0030, 0.0150},
	{"llama-3-70b", 0.0009, 0.0009},
	{"mixtral-8x7b", 0.0006, 0.0006},
	{"gpt-4.1-nano", 0.00010, 0.00040},
}

func handlePlaygroundModels(w http.ResponseWriter, r *http.Request) {
	type modelEntry struct {
		Name            string  `json:"name"`
		InputCostPer1K  float64 `json:"input_cost_per_1k"`
		OutputCostPer1K float64 `json:"output_cost_per_1k"`
	}
	models := make([]modelEntry, len(playgroundModels))
	for i, m := range playgroundModels {
		models[i] = modelEntry{Name: m.Name, InputCostPer1K: m.InputCostPer1K, OutputCostPer1K: m.OutputCostPer1K}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

func handlePlaygroundSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req playgroundSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	requestID := fmt.Sprintf("playground-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339)
	team := req.Team
	if team == "" {
		team = "playground"
	}

	_, err := db.Exec(
		`INSERT INTO cost_entries (request_id, model, input_tokens, output_tokens, timestamp, team)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (request_id) DO NOTHING`,
		requestID, req.Model, req.InputTokens, req.OutputTokens, ts, team,
	)
	if err != nil {
		slog.Error("playground: insert error", "err", err)
		http.Error(w, fmt.Sprintf("insert error: %v", err), http.StatusInternalServerError)
		return
	}

	totalTokens := req.InputTokens + req.OutputTokens

	costEvent := map[string]interface{}{
		"type":          "cost",
		"request_id":    requestID,
		"model":         req.Model,
		"input_tokens":  req.InputTokens,
		"output_tokens": req.OutputTokens,
		"total_tokens":  totalTokens,
		"team":          team,
		"timestamp":     ts,
	}
	costData, _ := json.Marshal(costEvent)
	events.broadcast(sseEvent{Type: "cost", Data: costData})

	events.broadcast(sseEvent{Type: "alert", Data: mustJSON(map[string]string{
		"severity": "info",
		"message":  fmt.Sprintf("Playground: %d tokens consumed by %s (%s)", totalTokens, req.Model, team),
	})})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(playgroundSendResponse{
		RequestID:    requestID,
		Model:        req.Model,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		TotalTokens:  totalTokens,
		Team:         team,
		Message:      fmt.Sprintf("Injected %d tokens for %s — refresh dashboard to see live update", totalTokens, req.Model),
	})
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
