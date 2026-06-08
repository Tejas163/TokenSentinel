package main

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

var (
	metricsRequestsTotal   = expvar.NewInt("requests_total")
	metricsRequestsSuccess = expvar.NewInt("requests_success")
	metricsRequestsError   = expvar.NewInt("requests_error")
	metricsRetriesTotal    = expvar.NewInt("retries_total")
	metricsCircuitOpen     = expvar.NewInt("circuit_breaker_open")
	metricsCircuitClosed   = expvar.NewInt("circuit_breaker_closed")
	metricsUpstreamLatency = expvar.NewMap("upstream_latency_ms")
)

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
		target = enforceBudget(r.Context(), team, route.Providers, target)
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
