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
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if key == "" || key != authAPIKey {
			slog.Warn("auth failure", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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

