package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

var modelEquivalence = map[string][]string{
	"gpt-4":            {"claude-3-sonnet", "gpt-4o-mini", "gpt-3.5-turbo"},
	"gpt-4-turbo":      {"claude-3-sonnet", "gpt-4o-mini", "gpt-3.5-turbo"},
	"gpt-4o":           {"gpt-4o-mini", "claude-3-haiku", "gpt-3.5-turbo"},
	"gpt-4o-mini":      {"gpt-3.5-turbo", "llama-3-8b"},
	"gpt-3.5-turbo":    {"llama-3-8b"},
	"claude-3-opus":    {"claude-3-sonnet", "claude-3-haiku", "gpt-4o-mini"},
	"claude-3-sonnet":  {"claude-3-haiku", "gpt-4o-mini"},
	"claude-3-haiku":   {"llama-3-8b"},
	"gemini-1.5-pro":   {"gemini-1.5-flash", "gpt-4o-mini"},
	"gemini-1.5-flash": {"llama-3-8b"},
	"mistral-large":    {"mistral-small", "gpt-4o-mini"},
	"mistral-small":    {"llama-3-8b"},
	"llama-3-70b":      {"mixtral-8x7b", "llama-3-8b"},
	"llama-3-8b":       {},
	"mixtral-8x7b":     {"llama-3-8b"},
}

func findEquivalents(model string, providers []UpstreamConfig) []*UpstreamConfig {
	modelLower := strings.ToLower(model)
	equivNames, ok := modelEquivalence[modelLower]
	if !ok {
		return nil
	}

	providerByModel := map[string]*UpstreamConfig{}
	for i := range providers {
		providerByModel[strings.ToLower(providers[i].Model)] = &providers[i]
	}

	var result []*UpstreamConfig
	for _, name := range equivNames {
		if p, ok := providerByModel[strings.ToLower(name)]; ok {
			result = append(result, p)
		}
	}
	return result
}

func buildFallbackChain(target *UpstreamConfig, providers []UpstreamConfig) []UpstreamConfig {
	chain := []UpstreamConfig{*target}

	used := map[string]bool{target.URL: true}

	equiv := findEquivalents(target.Model, providers)
	for _, p := range equiv {
		if !used[p.URL] {
			chain = append(chain, *p)
			used[p.URL] = true
		}
	}

	var remaining []UpstreamConfig
	for _, p := range providers {
		if !used[p.URL] {
			remaining = append(remaining, p)
		}
	}

	for i := 0; i < len(remaining); i++ {
		bestIdx := i
		for j := i + 1; j < len(remaining); j++ {
			if cheapestScore(remaining[j].Model) > cheapestScore(remaining[bestIdx].Model) {
				bestIdx = j
			}
		}
		remaining[i], remaining[bestIdx] = remaining[bestIdx], remaining[i]
		chain = append(chain, remaining[i])
	}

	return chain
}

type providerHealthEntry struct {
	successes int
	failures  int
	lastCheck time.Time
}

var (
	providerHealthMu   sync.RWMutex
	providerHealthData = map[string]*providerHealthEntry{}
)

func recordProviderSuccess(url string) {
	providerHealthMu.Lock()
	h, ok := providerHealthData[url]
	if !ok {
		h = &providerHealthEntry{}
		providerHealthData[url] = h
	}
	h.successes++
	h.lastCheck = time.Now()
	providerHealthMu.Unlock()
}

func recordProviderFailure(url string) {
	providerHealthMu.Lock()
	h, ok := providerHealthData[url]
	if !ok {
		h = &providerHealthEntry{}
		providerHealthData[url] = h
	}
	h.failures++
	h.lastCheck = time.Now()
	providerHealthMu.Unlock()
}

func providerErrorRate(url string) float64 {
	providerHealthMu.RLock()
	h, ok := providerHealthData[url]
	providerHealthMu.RUnlock()
	if !ok || (h.successes+h.failures) == 0 {
		return 0
	}
	return float64(h.failures) / float64(h.successes+h.failures)
}

func proxyWithFallback(ctx context.Context, reqID string, chain []UpstreamConfig, method string, body []byte, headers http.Header) (int, []byte, *UpstreamConfig, error) {
	if len(chain) == 0 {
		return 0, nil, nil, fmt.Errorf("empty fallback chain")
	}

	var lastErr error

	for i, cfg := range chain {
		cbKey := fmt.Sprintf("cb:%s", cfg.URL)
		cb := getOrCreateCB(cbKey)
		if !cb.Allow() {
			slog.Debug("fallback: skipping provider with open CB", "provider", cfg.URL, "model", cfg.Model, "position", i)
			lastErr = fmt.Errorf("circuit breaker open for %s", cfg.URL)
			continue
		}

		callStart := time.Now()
		statusCode, respBody, err := proxyWithRetry(ctx, reqID, cfg.URL, method, body, headers)
		callLatency := time.Since(callStart)

		if err == nil {
			recordProviderSuccess(cfg.URL)
			cb.Success()
			if i > 0 {
				slog.Warn("fallback used", "request_id", reqID, "original", chain[0].URL, "original_model", chain[0].Model, "fallback", cfg.URL, "fallback_model", cfg.Model, "position", i, "total_chain", len(chain))
			}
			return statusCode, respBody, &cfg, nil
		}

		recordProviderFailure(cfg.URL)
		cb.Failure()
		lastErr = err

		slog.Warn("fallback: provider failed, trying next", "request_id", reqID, "provider", cfg.URL, "model", cfg.Model, "position", i, "total_chain", len(chain), "latency_ms", callLatency.Milliseconds(), "error", err)
	}

	return 0, nil, nil, fmt.Errorf("all %d providers in fallback chain failed: %w", len(chain), lastErr)
}
