package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	routeCache      sync.Map
	circuitBreakers sync.Map
)

func enforceBudget(ctx context.Context, team string, providers []UpstreamConfig, target *UpstreamConfig) *UpstreamConfig {
	usedRaw, err := rdb.Get(ctx, fmt.Sprintf("budget:team:%s:used", team)).Result()
	if err == redis.Nil {
		slog.Debug("budget: no usage record for team", "team", team)
		return target
	}
	if err != nil {
		slog.Error("budget: failed to read usage", "team", team, "error", err)
		return target
	}
	used, parseErr := strconv.ParseInt(usedRaw, 10, 64)
	if parseErr != nil || used < 0 {
		slog.Error("budget: invalid usage value", "team", team, "raw", usedRaw)
		return target
	}

	limitRaw, err2 := rdb.Get(ctx, fmt.Sprintf("budget:team:%s:limit", team)).Result()
	if err2 == redis.Nil {
		slog.Debug("budget: no limit set for team", "team", team)
		return target
	}
	if err2 != nil {
		slog.Error("budget: failed to read limit", "team", team, "error", err2)
		return target
	}
	limit, parseErr2 := strconv.ParseInt(limitRaw, 10, 64)
	if parseErr2 != nil || limit <= 0 {
		slog.Error("budget: invalid limit value", "team", team, "raw", limitRaw)
		return target
	}
	if used >= limit {
		cheapest := cheapestProvider(providers)
		if cheapest != nil && cheapest.URL != target.URL {
			slog.Warn("team over budget, routing to cheapest", "team", team, "used", used, "limit", limit, "original", target.URL, "original_model", target.Model, "cheapest", cheapest.URL, "cheapest_model", cheapest.Model)
			return cheapest
		}
	}
	return target
}

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

func cheapestScore(model string) int {
	lower := strings.ToLower(model)
	for i, m := range cheapModels {
		if strings.Contains(lower, m) {
			return len(cheapModels) - i
		}
	}
	return 0
}

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
