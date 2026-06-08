package main

import (
	"net/http"
)

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

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

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

type EscalationPolicy struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	AlertType      string `json:"alert_type"`
	Model          string `json:"model"`
	Severity       string `json:"severity"`
	TimeoutMinutes int    `json:"timeout_minutes"`
	WebhookURL     string `json:"webhook_url"`
	Enabled        bool   `json:"enabled"`
	CreatedAt      string `json:"created_at"`
}

type contextKey string

const orgCtxKey contextKey = "org_id"

type AnomalyEntry struct {
	ID          int       `json:"id,omitempty"`
	RequestID   string    `json:"request_id"`
	Model       string    `json:"model"`
	TotalTokens int       `json:"total_tokens"`
	Mean        float64   `json:"mean"`
	Stddev      float64   `json:"stddev"`
	ZScore      float64   `json:"z_score"`
	DetectedAt  string    `json:"detected_at,omitempty"`
	OrgID       string    `json:"org_id,omitempty"`
}

type ModelCost struct {
	Model          string  `json:"model"`
	TotalTokens    int     `json:"total_tokens"`
	TotalInput     int     `json:"total_input"`
	TotalOutput    int     `json:"total_output"`
	RequestCount   int     `json:"request_count"`
	AvgInput       float64 `json:"avg_input"`
	AvgOutput      float64 `json:"avg_output"`
	Currency       string  `json:"currency"`
	CurrencySymbol string  `json:"currency_symbol"`
}

type costTimePoint struct {
	Hour           string  `json:"hour"`
	Model          string  `json:"model"`
	Cost           float64 `json:"cost"`
	Tokens         int64   `json:"tokens"`
	Currency       string  `json:"currency"`
	CurrencySymbol string  `json:"currency_symbol"`
}

type modelPriceEntry struct {
	Input  float64
	Output float64
}

type serviceHealth struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

type healthAllResponse struct {
	Status   string          `json:"status"`
	Services []serviceHealth `json:"services"`
}