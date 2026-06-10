package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

var authAPIKey string

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

func extractKey(r *http.Request) string {
	if key := r.Header.Get("X-Api-Key"); key != "" {
		return key
	}
	if b := r.Header.Get("Authorization"); len(b) > 7 && strings.EqualFold(b[:7], "Bearer ") {
		return b[7:]
	}
	return ""
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authAPIKey != "" {
			key := extractKey(r)
			if key == "" || key != authAPIKey {
				slog.Warn("auth failure", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
				writeError(w, http.StatusUnauthorized, "unauthorized", "")
				return
			}
			next(w, r)
			return
		}

		key := extractKey(r)
		if key == "" {
			slog.Warn("auth failure: missing api key", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
			writeError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}

		ak, err := validateAPIKey(r.Context(), key)
		if err != nil {
			slog.Error("auth: redis error", "error", err)
			writeError(w, http.StatusInternalServerError, "auth unavailable", "")
			return
		}
		if ak == nil {
			slog.Warn("auth failure: invalid key", "key_prefix", key[:min(8, len(key))])
			writeError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
		if ak.Status != "active" {
			slog.Warn("auth failure: inactive key", "key_prefix", key[:min(8, len(key))], "status", ak.Status)
			writeError(w, http.StatusUnauthorized, "key is inactive", "")
			return
		}

		ctx := context.WithValue(r.Context(), teamKey, ak.Team)
		ctx = context.WithValue(ctx, apiKeyInfoKey, ak)
		next(w, r.WithContext(ctx))
	}
}

func writeError(w http.ResponseWriter, status int, msg, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg, Request: reqID})
}
