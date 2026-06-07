package main

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- Circuit Breaker Tests ---

func TestCircuitBreaker_ClosedOnInit(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	if !cb.Allow() {
		t.Error("expected closed breaker to allow")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	for i := 0; i < 3; i++ {
		cb.Failure()
	}
	if cb.Allow() {
		t.Error("expected open breaker to reject")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	cb.Failure()
	cb.Failure()
	cb.Success()
	for i := 0; i < 3; i++ {
		if !cb.Allow() {
			t.Fatalf("expected closed after success at iteration %d", i)
		}
		cb.Failure()
	}
	if cb.Allow() {
		t.Error("expected open after failures exceeded")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)
	cb.Failure()
	if cb.Allow() {
		t.Fatal("expected open immediately after failure")
	}
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Error("expected half-open after cooldown")
	}
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)
	cb.Failure()
	time.Sleep(60 * time.Millisecond)
	cb.Allow()
	cb.Success()
	for i := 0; i < 3; i++ {
		if !cb.Allow() {
			t.Fatalf("expected closed after half-open success at iteration %d", i)
		}
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)
	cb.Failure()
	time.Sleep(60 * time.Millisecond)
	cb.Allow()
	cb.Failure()
	if cb.Allow() {
		t.Error("expected open after half-open failure")
	}
}

func TestCircuitBreaker_ConcurrentSafe(t *testing.T) {
	cb := NewCircuitBreaker(5, 100*time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Failure()
		}()
	}
	wg.Wait()
	if cb.Allow() {
		t.Error("expected open after 20 concurrent failures")
	}
}

func TestCircuitBreaker_ThresholdEdgeCases(t *testing.T) {
	t.Run("threshold=1 opens immediately", func(t *testing.T) {
		cb := NewCircuitBreaker(1, time.Second)
		cb.Failure()
		if cb.Allow() {
			t.Error("expected open after 1 failure")
		}
	})

	t.Run("threshold=0 opens on first failure", func(t *testing.T) {
		cb := NewCircuitBreaker(0, time.Second)
		cb.Failure()
		if cb.Allow() {
			t.Error("expected open after 1 failure with threshold=0")
		}
	})
}

// --- selectProvider Tests ---

func TestSelectProvider_NilOrEmpty(t *testing.T) {
	if got := selectProvider(nil); got != nil {
		t.Error("expected nil for nil providers")
	}
	if got := selectProvider([]UpstreamConfig{}); got != nil {
		t.Error("expected nil for empty providers")
	}
}

func TestSelectProvider_SingleProvider(t *testing.T) {
	providers := []UpstreamConfig{{URL: "http://example.com", Weight: 1}}
	got := selectProvider(providers)
	if got == nil || got.URL != "http://example.com" {
		t.Errorf("expected single provider, got %v", got)
	}
}

func TestSelectProvider_WeightedDistribution(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "heavy", Weight: 100},
		{URL: "light", Weight: 1},
	}
	counts := map[string]int{}
	iterations := 10000
	for i := 0; i < iterations; i++ {
		p := selectProvider(providers)
		if p == nil {
			t.Fatal("unexpected nil provider")
		}
		counts[p.URL]++
	}
	ratio := float64(counts["heavy"]) / float64(counts["light"])
	if ratio < 20 || ratio > 200 {
		t.Errorf("unexpected heavy/light ratio %.2f (expect ~100)", ratio)
	}
}

func TestSelectProvider_AllZeroWeightsFallback(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "a", Weight: 0},
		{URL: "b", Weight: 0},
	}
	for i := 0; i < 100; i++ {
		p := selectProvider(providers)
		if p == nil {
			t.Fatal("unexpected nil with zero-weight providers")
		}
	}
}

func TestSelectProvider_NegativeWeight(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "a", Weight: -5},
		{URL: "b", Weight: 10},
	}
	for i := 0; i < 100; i++ {
		p := selectProvider(providers)
		if p == nil {
			t.Fatal("unexpected nil with negative weight")
		}
	}
}

func TestSelectProvider_DeterministicSeed(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "a", Weight: 1},
		{URL: "b", Weight: 1},
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p := selectProvider(providers)
		seen[p.URL] = true
	}
	if len(seen) < 2 {
		t.Error("expected both providers to be selected eventually with equal weights")
	}
}

// --- writeError Tests ---

func TestWriteError_ResponseFormat(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadGateway, "upstream error", "test-req-id")
	resp := w.Result()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestWriteError_AllStatusCodes(t *testing.T) {
	codes := []int{
		http.StatusNotFound,
		http.StatusBadRequest,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	}
	for _, code := range codes {
		w := httptest.NewRecorder()
		writeError(w, code, "test error", "")
		if w.Result().StatusCode != code {
			t.Errorf("expected status %d, got %d", code, w.Result().StatusCode)
		}
	}
}

// --- getHTTPClient Tests ---

func TestGetHTTPClient_CachesByTimeout(t *testing.T) {
	providerClients = sync.Map{}
	c1 := getHTTPClient(10)
	c2 := getHTTPClient(10)
	c3 := getHTTPClient(30)
	if c1 != c2 {
		t.Error("expected same client for same timeout")
	}
	if c1 == c3 {
		t.Error("expected different client for different timeout")
	}
}

func TestGetHTTPClient_TimeoutSet(t *testing.T) {
	providerClients = sync.Map{}
	c := getHTTPClient(5)
	if c.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", c.Timeout)
	}
}

// --- proxyWithRetry Tests ---

func TestProxyWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	code, body, err := proxyWithRetry(context.Background(), "test-req", srv.URL, "GET", nil, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("expected 200, got %d", code)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestProxyWithRetry_RetriesOnServerError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	code, _, err := proxyWithRetry(context.Background(), "test-retry", srv.URL, "GET", nil, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusInternalServerError {
		t.Errorf("expected 500 after retries, got %d", code)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
}

func TestProxyWithRetry_RetriesOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	code, _, err := proxyWithRetry(context.Background(), "test-429", srv.URL, "GET", nil, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after retries, got %d", code)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestProxyWithRetry_RetriesOn503(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	code, _, err := proxyWithRetry(context.Background(), "test-503", srv.URL, "GET", nil, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after retries, got %d", code)
	}
}

func TestProxyWithRetry_EventuallySucceeds(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	code, body, err := proxyWithRetry(context.Background(), "test-eventual", srv.URL, "GET", nil, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("expected 200, got %d", code)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("unexpected body: %s", body)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestProxyWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := proxyWithRetry(ctx, "test-ctx", "http://nonexistent/", "GET", nil, http.Header{})
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestProxyWithRetry_RequestIDForwarded(t *testing.T) {
	var gotReqID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReqID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	headers := http.Header{}
	headers.Set("X-Request-ID", "custom-id")
	proxyWithRetry(context.Background(), "custom-id", srv.URL, "GET", nil, headers)
	if gotReqID != "custom-id" {
		t.Errorf("expected X-Request-ID=custom-id, got %s", gotReqID)
	}
}

// --- getOrCreateCB Tests ---

func TestGetOrCreateCB_Caches(t *testing.T) {
	circuitBreakers = sync.Map{}
	cb1 := getOrCreateCB("test-key")
	cb2 := getOrCreateCB("test-key")
	if cb1 != cb2 {
		t.Error("expected same breaker for same key")
	}
}

func TestGetOrCreateCB_UniqueKeys(t *testing.T) {
	circuitBreakers = sync.Map{}
	cb1 := getOrCreateCB("key-a")
	cb2 := getOrCreateCB("key-b")
	if cb1 == cb2 {
		t.Error("expected different breakers for different keys")
	}
}

// --- Helper to reset global state between tests ---

func resetGlobals() {
	providerClients = sync.Map{}
	circuitBreakers = sync.Map{}
	rand.Seed(time.Now().UnixNano())
}

func init() {
	resetGlobals()
}
