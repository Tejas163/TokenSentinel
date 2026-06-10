package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"log/slog"
)

func metricsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next(wrapped, r)
		duration := time.Since(start).Seconds()
		httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, fmt.Sprintf("%d", wrapped.statusCode)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	}
}

func extractAuthKey(r *http.Request) string {
	if key := r.Header.Get("X-Api-Key"); key != "" {
		return key
	}
	if b := r.Header.Get("Authorization"); len(b) > 7 && strings.EqualFold(b[:7], "Bearer ") {
		return b[7:]
	}
	return r.URL.Query().Get("api_key")
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := extractAuthKey(r)
		if key == "" {
			slog.Warn("auth failure: missing api key", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if authAPIKey != "" {
			if key != authAPIKey {
				slog.Warn("auth failure", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
			return
		}

		ak, err := validateDashboardKey(r.Context(), key)
		if err != nil {
			slog.Error("auth: redis error", "error", err)
			http.Error(w, "auth unavailable", http.StatusInternalServerError)
			return
		}
		if ak == nil {
			slog.Warn("auth failure: invalid key", "method", r.Method, "path", r.URL.Path)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if ak.Status != "active" {
			slog.Warn("auth failure: inactive key", "status", ak.Status)
			http.Error(w, "key is inactive", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func orgMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		orgID := r.Header.Get("X-Org-Id")
		if orgID == "" {
			orgID = r.URL.Query().Get("org_id")
		}
		if orgID != "" {
			ctx := context.WithValue(r.Context(), orgCtxKey, orgID)
			r = r.WithContext(ctx)
		}
		next(w, r)
	})
}
