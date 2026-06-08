package main

import (
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

func writeError(w http.ResponseWriter, status int, msg, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg, Request: reqID})
}
