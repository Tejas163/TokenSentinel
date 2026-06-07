package main

import (
	"bytes"
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client
var defaultPool *workerPool

type contextKey string

const reqIDKey contextKey = "request_id"

var authAPIKey string

var sensitiveHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
	"x-api-key":     true,
	"proxy-authorization": true,
}

func isSensitiveHeader(k string) bool {
	return sensitiveHeaders[k]
}

var (
	metricsRequestsTotal   = expvar.NewInt("requests_total")
	metricsRequestsSuccess = expvar.NewInt("requests_success")
	metricsRequestsError   = expvar.NewInt("requests_error")
	metricsRetriesTotal    = expvar.NewInt("retries_total")
	metricsCircuitOpen     = expvar.NewInt("circuit_breaker_open")
	metricsCircuitClosed   = expvar.NewInt("circuit_breaker_closed")
	metricsUpstreamLatency = expvar.NewMap("upstream_latency_ms")
)

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
	AutoModel bool             `json:"auto_model"`
}

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	authAPIKey = os.Getenv("AUTH_API_KEY")

	initSemanticCache(rdb)
	initRateLimiter()
	defaultPool = newWorkerPool(0)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", authMiddleware(healthHandler))
	mux.HandleFunc("/metrics", authMiddleware(metricsHandler))
	mux.HandleFunc("/", authMiddleware(rateLimitMiddleware(proxyHandler)))

	slog.Info("starting server", "addr", ":8080", "redis", redisAddr, "auth_enabled", authAPIKey != "")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	expvar.Handler().ServeHTTP(w, r)
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

func getReqID(r *http.Request) string {
	if id, ok := r.Context().Value(reqIDKey).(string); ok && id != "" {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authAPIKey == "" {
			next(w, r)
			return
		}
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			if b := r.Header.Get("Authorization"); len(b) > 7 && strings.EqualFold(b[:7], "Bearer ") {
				key = b[7:]
			}
		}
		if key == "" || key != authAPIKey {
			slog.Warn("auth failure", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
			writeError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
		next(w, r)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = fmt.Sprintf("gen-%d", time.Now().UnixNano())
	}
	start := time.Now()
	metricsRequestsTotal.Add(1)

	route, err := resolveRoute(r.Context(), r.URL.Path)
	if err != nil {
		metricsRequestsError.Add(1)
		slog.Error("route resolution failed", "request_id", reqID, "path", r.URL.Path, "error", err)
		writeError(w, http.StatusBadGateway, "route resolution failed", reqID)
		return
	}
	if route == nil {
		metricsRequestsError.Add(1)
		slog.Warn("no route for path", "request_id", reqID, "path", r.URL.Path)
		writeError(w, http.StatusNotFound, "no route for path", reqID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		metricsRequestsError.Add(1)
		slog.Error("failed to read request body", "request_id", reqID, "error", err)
		if err.Error() == "http: request body too large" {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 10MB limit", reqID)
		} else {
			writeError(w, http.StatusBadRequest, "failed to read request body", reqID)
		}
		return
	}
	defer r.Body.Close()

	if semanticCache != nil {
		promptText := extractPromptText(body)
		if cached, err := semanticCache.Lookup(r.Context(), promptText); err == nil && cached != nil {
			inputTokens := estimateTokens(string(body), cached.Model)
			savingsCents := estimateCost(inputTokens, cached.OutputTokens, cached.Model)
			slog.Info("semantic cache hit", "request_id", reqID, "model", cached.Model, "savings_cents", savingsCents, "similarity", "cached")
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("X-Model-Used", cached.Model)
			w.Header().Set("X-Cost-Cents", "0.00")
			w.Header().Set("X-Cache-Savings-Cents", fmt.Sprintf("%.2f", savingsCents))
			w.WriteHeader(cached.StatusCode)
			w.Write(cached.Body)
			return
		}
	}

	target := selectProvider(route.Providers)
	if route.AutoModel {
		target = selectModel(body, route.Providers)
	}
	if target == nil {
		metricsRequestsError.Add(1)
		slog.Error("no available providers", "request_id", reqID, "path", r.URL.Path)
		writeError(w, http.StatusServiceUnavailable, "no available providers", reqID)
		return
	}

	if team := r.Header.Get("X-Team-Name"); team != "" {
		usedRaw, err := rdb.Get(r.Context(), fmt.Sprintf("budget:team:%s:used", team)).Result()
		if err == redis.Nil {
			slog.Debug("budget: no usage record for team", "team", team)
		} else if err != nil {
			slog.Error("budget: failed to read usage", "team", team, "error", err)
		} else {
			used, parseErr := strconv.ParseInt(usedRaw, 10, 64)
			if parseErr != nil || used < 0 {
				slog.Error("budget: invalid usage value", "team", team, "raw", usedRaw)
			} else {
				limitRaw, err2 := rdb.Get(r.Context(), fmt.Sprintf("budget:team:%s:limit", team)).Result()
				if err2 == redis.Nil {
					slog.Debug("budget: no limit set for team", "team", team)
				} else if err2 != nil {
					slog.Error("budget: failed to read limit", "team", team, "error", err2)
				} else {
					limit, parseErr2 := strconv.ParseInt(limitRaw, 10, 64)
					if parseErr2 != nil || limit <= 0 {
						slog.Error("budget: invalid limit value", "team", team, "raw", limitRaw)
					} else if used >= limit {
						cheapest := cheapestProvider(route.Providers)
						if cheapest != nil && cheapest.URL != target.URL {
							slog.Warn("team over budget, routing to cheapest", "team", team, "used", used, "limit", limit, "original", target.URL, "original_model", target.Model, "cheapest", cheapest.URL, "cheapest_model", cheapest.Model)
							target = cheapest
						}
					} else {
						slog.Debug("budget OK", "team", team, "used", used, "limit", limit)
					}
				}
			}
		}
	}

	ml := applyBodyLimitOverride(getModelLimit(target.Model))
	inputTokens := estimateTokens(string(body), target.Model)
	if inputTokens > ml.maxInputTokens {
		metricsRequestsError.Add(1)
		slog.Error("input exceeds model context window", "request_id", reqID, "model", target.Model, "input_tokens", inputTokens, "limit", ml.maxInputTokens)
		writeError(w, http.StatusBadRequest, fmt.Sprintf("input of %d tokens exceeds model %s limit of %d tokens", inputTokens, target.Model, ml.maxInputTokens), reqID)
		return
	}

	chain := buildFallbackChain(target, route.Providers)
	statusCode, respBody, usedProvider, err := proxyWithFallback(r.Context(), reqID, chain, r.Method, body, r.Header)
	latency := time.Since(start).Milliseconds()
	metricsUpstreamLatency.Add(fmt.Sprintf("%dms", latency/100*100), 1)

	if err != nil {
		metricsRequestsError.Add(1)
		slog.Error("upstream error", "request_id", reqID, "provider", target.URL, "error", err, "latency_ms", latency)
		writeError(w, http.StatusBadGateway, "upstream error", reqID)
		return
	}

	if usedProvider != nil && usedProvider.URL != target.URL {
		slog.Warn("fallback provider succeeded", "request_id", reqID, "original", target.URL, "original_model", target.Model, "used", usedProvider.URL, "used_model", usedProvider.Model)
		target = usedProvider
		w.Header().Set("X-Cost-Routing", "fallback")
	}

	metricsRequestsSuccess.Add(1)
	slog.Info("request complete", "request_id", reqID, "provider", target.URL, "model", target.Model, "status", statusCode, "latency_ms", latency)

	inputTokens = estimateTokens(string(body), target.Model)
	outputTokens := estimateTokens(string(respBody), target.Model)
	defaultPool.Submit(costTask{
		reqID:        reqID,
		model:        target.Model,
		inputTokens:  inputTokens,
		outputTokens: outputTokens,
	})

	costCents := estimateCost(inputTokens, outputTokens, target.Model)

	if semanticCache != nil {
		promptText := extractPromptText(body)
		if statusCode >= 200 && statusCode < 300 && promptText != "" {
			defaultPool.Submit(cacheTask{
				promptText: promptText,
				resp: &CachedResponse{
					Body:         respBody,
					Model:        target.Model,
					StatusCode:   statusCode,
					OutputTokens: outputTokens,
					CachedAt:     time.Now().Unix(),
				},
			})
		}
		w.Header().Set("X-Cache", "MISS")
	}

	w.Header().Set("X-Model-Used", target.Model)
	w.Header().Set("X-Cost-Cents", fmt.Sprintf("%.2f", costCents))
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

type routeCacheEntry struct {
	cfg    *RouteConfig
	expiry time.Time
}

var (
	routeCache      sync.Map
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
	if cached, ok := routeCache.Load(path); ok {
		entry := cached.(routeCacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.cfg, nil
		}
		routeCache.Delete(path)
	}

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
	routeCache.Store(path, routeCacheEntry{cfg: &cfg, expiry: time.Now().Add(60 * time.Second)})
	return &cfg, nil
}

var cheapModels = []string{"gpt-3.5", "claude-3-haiku", "llama-3-8b", "mistral-small", "gemini-1.5-flash"}

func cheapestProvider(providers []UpstreamConfig) *UpstreamConfig {
	if len(providers) == 0 {
		return nil
	}
	best := &providers[0]
	bestScore := cheapestScore(best.Model)
	for i := 1; i < len(providers); i++ {
		s := cheapestScore(providers[i].Model)
		if s > bestScore {
			best = &providers[i]
			bestScore = s
		}
	}
	return best
}

func cheapestScore(model string) int {
	lower := strings.ToLower(model)
	for i, m := range cheapModels {
		if strings.Contains(lower, m) {
			return len(cheapModels) - i
		}
	}
	return 0
}

func selectProvider(providers []UpstreamConfig) *UpstreamConfig {
	if len(providers) == 0 {
		return nil
	}
	if len(providers) == 1 {
		return &providers[0]
	}

	totalWeight := 0
	weights := make([]int, len(providers))
	for i, p := range providers {
		w := adaptiveWeight(p)
		weights[i] = w
		totalWeight += w
	}

	if totalWeight <= 0 {
		return &providers[0]
	}

	logAdaptiveWeights(providers)

	roll := rand.Intn(totalWeight)
	cumulative := 0
	for i := range providers {
		cumulative += weights[i]
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
			metricsRetriesTotal.Add(1)
			slog.Info("retrying request", "request_id", reqID, "attempt", attempt, "max_retries", maxRetries, "backoff_ms", (delay+jitter).Milliseconds())
		}

		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return 0, nil, fmt.Errorf("failed to create request: %w", err)
		}

		for k, vals := range headers {
			if isSensitiveHeader(k) {
				continue
			}
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
		"team":          os.Getenv("BUDGET_TEAM_NAME"),
	}
	data, _ := json.Marshal(entry)

	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(50*(1<<attempt)) * time.Millisecond)
		}
		pipe := rdb.Pipeline()
		pipe.Set(ctx, costKey, data, 24*time.Hour)
		pipe.Publish(ctx, "health:events", fmt.Sprintf("cost:%s", reqID))
		_, err := pipe.Exec(ctx)
		if err == nil {
			return
		}
		slog.Error("failed to record cost", "request_id", reqID, "attempt", attempt+1, "max_retries", maxRetries, "error", err)
	}
	slog.Warn("cost write failed after all retries, logging to stderr", "request_id", reqID, "model", model, "input_tokens", inputTokens, "output_tokens", outputTokens, "data", string(data))
}

	type ErrorResponse struct {
		Error     string `json:"error"`
		Request   string `json:"request_id,omitempty"`
		ErrorCode string `json:"error_code,omitempty"`
	}

	func writeError(w http.ResponseWriter, status int, msg, reqID string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(ErrorResponse{Error: msg, Request: reqID})
	}
