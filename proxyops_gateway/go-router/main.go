package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

type CircuitState int32

const (
	Closed CircuitState = iota
	Open
	HalfOpen
)

type CircuitBreaker struct {
	state     atomic.Int32
	failures  atomic.Int32
	threshold int32
	cooldown  time.Duration
	mu        sync.Mutex
	openSince time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	cb := &CircuitBreaker{threshold: int32(threshold), cooldown: cooldown}
	cb.state.Store(int32(Closed))
	return cb
}

func (cb *CircuitBreaker) Allow() bool {
	state := CircuitState(cb.state.Load())
	if state == Open {
		cb.mu.Lock()
		defer cb.mu.Unlock()
		if time.Since(cb.openSince) > cb.cooldown {
			cb.state.Store(int32(HalfOpen))
			return true
		}
		return false
	}
	return true
}

func (cb *CircuitBreaker) Success() {
	cb.state.Store(int32(Closed))
	cb.failures.Store(0)
}

func (cb *CircuitBreaker) Failure() {
	f := cb.failures.Add(1)
	if f >= cb.threshold {
		cb.mu.Lock()
		cb.state.Store(int32(Open))
		cb.openSince = time.Now()
		cb.mu.Unlock()
	}
}

type UpstreamConfig struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
	Model   string `json:"model"`
	Weight  int    `json:"weight"`
}

type RouteConfig struct {
	Pattern   string           `json:"pattern"`
	Providers []UpstreamConfig `json:"providers"`
}

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/", proxyHandler)

	log.Println("Go Router starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err := rdb.Ping(ctx).Err()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "error"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

var providerClients = sync.Map{}

func getHTTPClient(timeout int) *http.Client {
	key := fmt.Sprintf("%d", timeout)
	if cached, ok := providerClients.Load(key); ok {
		return cached.(*http.Client)
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	providerClients.Store(key, client)
	return client
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = fmt.Sprintf("gen-%d", time.Now().UnixNano())
	}

	route, err := resolveRoute(r.Context(), r.URL.Path)
	if err != nil {
		log.Printf("[%s] route resolution failed: %v", reqID, err)
		writeError(w, http.StatusBadGateway, "route resolution failed")
		return
	}
	if route == nil {
		log.Printf("[%s] no route for path: %s", reqID, r.URL.Path)
		writeError(w, http.StatusNotFound, "no route for path")
		return
	}

	target := selectProvider(route.Providers)
	if target == nil {
		log.Printf("[%s] no available providers", reqID)
		writeError(w, http.StatusServiceUnavailable, "no available providers")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[%s] failed to read request body: %v", reqID, err)
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	cbKey := fmt.Sprintf("cb:%s", target.URL)
	cb := getOrCreateCB(cbKey)

	if !cb.Allow() {
		log.Printf("[%s] circuit breaker open for %s", reqID, target.URL)
		writeError(w, http.StatusServiceUnavailable, "provider temporarily unavailable")
		return
	}

	statusCode, respBody, err := proxyWithRetry(r.Context(), reqID, target.URL, r.Method, body, r.Header)
	if err != nil {
		cb.Failure()
		log.Printf("[%s] upstream error for %s: %v", reqID, target.URL, err)
		writeError(w, http.StatusBadGateway, "upstream error")
		return
	}

	cb.Success()

	go recordCost(context.Background(), reqID, target.Model, len(body), len(respBody))

	for k, vals := range r.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

var (
	circuitBreakers sync.Map
)

func getOrCreateCB(key string) *CircuitBreaker {
	if cb, ok := circuitBreakers.Load(key); ok {
		return cb.(*CircuitBreaker)
	}
	cb := NewCircuitBreaker(5, 30*time.Second)
	circuitBreakers.Store(key, cb)
	return cb
}

func resolveRoute(ctx context.Context, path string) (*RouteConfig, error) {
	pattern := fmt.Sprintf("routes:%s", path)
	val, err := rdb.Get(ctx, pattern).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg RouteConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func selectProvider(providers []UpstreamConfig) *UpstreamConfig {
	if len(providers) == 0 {
		return nil
	}
	if len(providers) == 1 {
		return &providers[0]
	}

	totalWeight := 0
	for _, p := range providers {
		if p.Weight <= 0 {
			p.Weight = 1
		}
		totalWeight += p.Weight
	}

	roll := rand.Intn(totalWeight)
	cumulative := 0
	for i := range providers {
		w := providers[i].Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if roll < cumulative {
			return &providers[i]
		}
	}
	return &providers[len(providers)-1]
}

func proxyWithRetry(ctx context.Context, reqID, target string, method string, body []byte, headers http.Header) (int, []byte, error) {
	maxRetries := 2
	baseDelay := 200 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<(attempt-1))
			jitter := time.Duration(rand.Intn(100)) * time.Millisecond
			time.Sleep(delay + jitter)
			log.Printf("[%s] retry attempt %d/%d after %v", reqID, attempt, maxRetries, delay+jitter)
		}

		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return 0, nil, fmt.Errorf("failed to create request: %w", err)
		}

		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("X-Request-ID", reqID)

		timeout := 30
		client := getHTTPClient(timeout)

		resp, err := client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return 0, nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return 0, nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			if attempt < maxRetries {
				continue
			}
		}

		if resp.StatusCode >= 500 && attempt < maxRetries {
			continue
		}

		return resp.StatusCode, respBody, nil
	}

	return 0, nil, fmt.Errorf("max retries exceeded")
}

func recordCost(ctx context.Context, reqID, model string, inputTokens, outputTokens int) {
	costKey := fmt.Sprintf("sentinel:%s:cost", reqID)
	entry := map[string]interface{}{
		"model":         model,
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(entry)
	pipe := rdb.Pipeline()
	pipe.Set(ctx, costKey, data, 24*time.Hour)
	pipe.Publish(ctx, "health:events", fmt.Sprintf("cost:%s", reqID))
	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("[%s] failed to record cost: %v", reqID, err)
	}
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Request string `json:"request_id,omitempty"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}
