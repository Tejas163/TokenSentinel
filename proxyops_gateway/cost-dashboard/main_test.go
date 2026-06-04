package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTest(t *testing.T) (sqlmock.Sqlmock, *miniredis.Miniredis) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	db = mockDB

	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb = redis.NewClient(&redis.Options{Addr: srv.Addr()})

	authAPIKey = "test-key-123"
	events = newSSEBroker()

	t.Cleanup(func() {
		mockDB.Close()
		srv.Close()
		rdb.Close()
	})
	return mock, srv
}

func request(method, target string, body ...string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	if len(body) > 0 {
		r.Body = nil
	}
	return r
}

func withKey(r *http.Request, key string) *http.Request {
	r.Header.Set("X-Api-Key", key)
	return r
}

// ---------------------------------------------------------------------------
// authMiddleware
// ---------------------------------------------------------------------------

func TestAuthMiddleware_passthroughWhenKeyEmpty(t *testing.T) {
	authAPIKey = ""
	defer func() { authAPIKey = "test-key-123" }()

	var called bool
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	h(w, request("GET", "/api/dashboard/costs"))
	if !called {
		t.Fatal("expected handler to be called")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_validXApiKey(t *testing.T) {
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	h(w, withKey(request("GET", "/api/dashboard/costs"), "test-key-123"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_validBearer(t *testing.T) {
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := request("GET", "/api/dashboard/costs")
	r.Header.Set("Authorization", "Bearer test-key-123")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_validQueryParam(t *testing.T) {
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	h(w, request("GET", "/api/dashboard/costs?api_key=test-key-123"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_missingKeyReturns401(t *testing.T) {
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})

	w := httptest.NewRecorder()
	h(w, request("GET", "/api/dashboard/costs"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "unauthorized") {
		t.Fatalf("expected unauthorized body, got %s", body)
	}
}

func TestAuthMiddleware_wrongKeyReturns401(t *testing.T) {
	h := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})

	w := httptest.NewRecorder()
	h(w, withKey(request("GET", "/api/dashboard/costs"), "wrong-key"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleDashboardHealth
// ---------------------------------------------------------------------------

func TestHandleDashboardHealth_ok(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	w := httptest.NewRecorder()
	handleDashboardHealth(w, request("GET", "/health"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected ok, got %v", resp["status"])
	}
}

func TestHandleDashboardHealth_degraded(t *testing.T) {
	mock, srv := setupTest(t)
	defer mock.ExpectClose()
	srv.Close()

	w := httptest.NewRecorder()
	handleDashboardHealth(w, request("GET", "/health"))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp["status"] != "degraded" {
		t.Fatalf("expected degraded, got %v", resp["status"])
	}
}

// ---------------------------------------------------------------------------
// handleCosts
// ---------------------------------------------------------------------------

func TestHandleCosts_success(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT model, SUM`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total_tokens", "total_input", "total_output", "request_count"}).
			AddRow("gpt-4", 1000, 600, 400, 5).
			AddRow("claude-3", 500, 300, 200, 2))

	w := httptest.NewRecorder()
	handleCosts(w, request("GET", "/api/dashboard/costs?period=24h"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var results []ModelCost
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 models, got %d", len(results))
	}
	if results[0].Model != "gpt-4" || results[0].RequestCount != 5 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestHandleCosts_empty(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT model, SUM`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnRows(sqlmock.NewRows([]string{"model", "total_tokens", "total_input", "total_output", "request_count"}))

	w := httptest.NewRecorder()
	handleCosts(w, request("GET", "/api/dashboard/costs?period=1h"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var results []ModelCost
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty, got %d", len(results))
	}
}

func TestHandleCosts_invalidPeriodReturns400(t *testing.T) {
	_, _ = setupTest(t)

	w := httptest.NewRecorder()
	handleCosts(w, request("GET", "/api/dashboard/costs?period=xyz"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCosts_dbErrorReturns500(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT model, SUM`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnError(fmt.Errorf("connection refused"))

	w := httptest.NewRecorder()
	handleCosts(w, request("GET", "/api/dashboard/costs?period=24h"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleSummary
// ---------------------------------------------------------------------------

func TestHandleSummary_success(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count", "total_tokens", "total_input", "total_output", "unique_models"}).
			AddRow(150, 50000, 30000, 20000, 4))

	w := httptest.NewRecorder()
	handleSummary(w, request("GET", "/api/dashboard/summary?period=24h"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var s struct {
		TotalRequests int     `json:"total_requests"`
		TotalTokens   int     `json:"total_tokens"`
		UniqueModels  int     `json:"unique_models"`
		AvgTokensPer  float64 `json:"avg_tokens_per_request"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("json: %v", err)
	}
	if s.TotalRequests != 150 || s.UniqueModels != 4 {
		t.Fatalf("unexpected summary: %+v", s)
	}
	if s.AvgTokensPer != 333.3333333333333 {
		t.Fatalf("expected ~333.33 avg tokens, got %f", s.AvgTokensPer)
	}
}

func TestHandleSummary_empty(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count", "total_tokens", "total_input", "total_output", "unique_models"}).
			AddRow(0, 0, 0, 0, 0))

	w := httptest.NewRecorder()
	handleSummary(w, request("GET", "/api/dashboard/summary?period=24h"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var s struct {
		TotalRequests int `json:"total_requests"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("json: %v", err)
	}
	if s.TotalRequests != 0 {
		t.Fatalf("expected 0 requests, got %d", s.TotalRequests)
	}
}

func TestHandleSummary_invalidPeriodReturns400(t *testing.T) {
	_, _ = setupTest(t)

	w := httptest.NewRecorder()
	handleSummary(w, request("GET", "/api/dashboard/summary?period=abc"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSummary_dbErrorReturns500(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(fmt.Errorf("timeout"))

	w := httptest.NewRecorder()
	handleSummary(w, request("GET", "/api/dashboard/summary?period=24h"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleAnomalies
// ---------------------------------------------------------------------------

func TestHandleAnomalies_success(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT id, request_id, model, total_tokens, mean, stddev, z_score, detected_at
		FROM anomalies`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "model", "total_tokens", "mean", "stddev", "z_score", "detected_at"}).
			AddRow(1, "req-001", "gpt-4", 5000, 1000, 200, 20.0, time.Now().Add(-1*time.Hour)).
			AddRow(2, "req-002", "claude-3", 3000, 800, 150, 14.67, time.Now().Add(-30*time.Minute)))

	w := httptest.NewRecorder()
	handleAnomalies(w, request("GET", "/api/dashboard/anomalies?period=24h"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var results []AnomalyEntry
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(results))
	}
	if results[0].RequestID != "req-001" || results[0].ZScore != 20.0 {
		t.Fatalf("unexpected first anomaly: %+v", results[0])
	}
}

func TestHandleAnomalies_empty(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT id, request_id, model, total_tokens, mean, stddev, z_score, detected_at
		FROM anomalies`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "model", "total_tokens", "mean", "stddev", "z_score", "detected_at"}))

	w := httptest.NewRecorder()
	handleAnomalies(w, request("GET", "/api/dashboard/anomalies?period=24h"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var results []AnomalyEntry
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty, got %d", len(results))
	}
}

func TestHandleAnomalies_invalidPeriodFallsBack(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT id, request_id, model, total_tokens, mean, stddev, z_score, detected_at
		FROM anomalies`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "model", "total_tokens", "mean", "stddev", "z_score", "detected_at"}))

	w := httptest.NewRecorder()
	handleAnomalies(w, request("GET", "/api/dashboard/anomalies?period=forever"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fallback to 168h), got %d", w.Code)
	}
}

func TestHandleAnomalies_dbErrorReturns500(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	mock.ExpectQuery(`SELECT id, request_id, model, total_tokens, mean, stddev, z_score, detected_at
		FROM anomalies`).
		WithArgs(sqlmock.AnyArg(), 100, 0).
		WillReturnError(fmt.Errorf("disk full"))

	w := httptest.NewRecorder()
	handleAnomalies(w, request("GET", "/api/dashboard/anomalies?period=24h"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleSSE
// ---------------------------------------------------------------------------

func TestHandleSSE_receivesEvent(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	events = newSSEBroker()

	w := httptest.NewRecorder()
	r := request("GET", "/api/dashboard/events")
	ctx, cancel := withDeadline(r, 50*time.Millisecond)
	defer cancel()
	*r = *r.WithContext(ctx)

	go func() {
		time.Sleep(5 * time.Millisecond)
		events.broadcast(sseEvent{Type: "cost", Data: []byte(`{"test":true}`)})
	}()

	handleSSE(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "event: cost") {
		t.Fatalf("expected event: cost in body, got: %s", body)
	}
	if !strings.Contains(body, `{"test":true}`) {
		t.Fatalf("expected data payload in body, got: %s", body)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected event-stream content type")
	}
}

func TestHandleSSE_noFlusherReturns500(t *testing.T) {
	mock, _ := setupTest(t)
	defer mock.ExpectClose()

	events = newSSEBroker()

	w := httptest.NewRecorder()
	nfw := &noFlusherResponseWriter{ResponseWriter: w}
	r := request("GET", "/api/dashboard/events")
	handleSSE(nfw, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type noFlusherResponseWriter struct {
	http.ResponseWriter
}

func withDeadline(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithDeadline(r.Context(), time.Now().Add(d))
}
