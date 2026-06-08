package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var webhookClient = &http.Client{Timeout: 10 * time.Second}

var (
	lastNotified   = make(map[int]time.Time)
	lastNotifiedMu sync.Mutex
)

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}

func lookupEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		if fallback == "" {
			fatal("required env var not set", fmt.Errorf("key=%s", key))
		}
		slog.Info("env not set, using default", "key", key, "fallback", fallback)
		return fallback
	}
	return v
}

func getOrgID(r *http.Request) string {
	if v := r.Context().Value(orgCtxKey); v != nil {
		return v.(string)
	}
	return ""
}

func signPayload(payload []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func signAndPost(url string, payload []byte) (*http.Response, error) {
	sig := signPayload(payload, authAPIKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TokenSentinel-Signature", sig)
	return webhookClient.Do(req)
}

func modelPrice(model string) modelPriceEntry {
	for _, entry := range modelCatalog {
		if strings.HasPrefix(strings.ToLower(model), strings.ToLower(entry.prefix)) {
			return entry.price
		}
	}
	return modelPriceEntry{Input: 3.00, Output: 15.00}
}

var modelCatalog = []struct {
	prefix string
	price  modelPriceEntry
}{
	{"gpt-4o", modelPriceEntry{2.50, 10.00}},
	{"gpt-4-turbo", modelPriceEntry{10.00, 30.00}},
	{"gpt-4", modelPriceEntry{30.00, 60.00}},
	{"gpt-4o-mini", modelPriceEntry{0.15, 0.60}},
	{"gpt-3.5", modelPriceEntry{0.50, 1.50}},
	{"claude-3-opus", modelPriceEntry{15.00, 75.00}},
	{"claude-3-sonnet", modelPriceEntry{3.00, 15.00}},
	{"claude-3-haiku", modelPriceEntry{0.25, 1.25}},
	{"claude-3.5-sonnet", modelPriceEntry{3.00, 15.00}},
	{"claude-3.5-haiku", modelPriceEntry{0.25, 1.25}},
	{"llama-3-8b", modelPriceEntry{0.05, 0.20}},
	{"llama-3-70b", modelPriceEntry{0.59, 0.79}},
	{"llama-3.1-405b", modelPriceEntry{2.50, 10.00}},
	{"mixtral-8x7b", modelPriceEntry{0.24, 0.72}},
	{"mistral-large", modelPriceEntry{2.00, 6.00}},
	{"mistral-medium", modelPriceEntry{0.70, 2.10}},
	{"mistral-small", modelPriceEntry{0.20, 0.60}},
	{"gemini-1.5-pro", modelPriceEntry{1.25, 5.00}},
	{"gemini-1.5-flash", modelPriceEntry{0.075, 0.30}},
	{"gemini-2.0-pro", modelPriceEntry{2.50, 10.00}},
	{"gemini-2.0-flash", modelPriceEntry{0.10, 0.40}},
}