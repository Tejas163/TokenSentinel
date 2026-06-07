package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
}

type rateLimiter struct {
	mu       sync.RWMutex
	buckets  map[string]*tokenBucket
	capacity float64
	refill   float64
}

var globalRateLimiter *rateLimiter

func initRateLimiter() {
	capacity := 60.0
	refill := 60.0
	if s := os.Getenv("RATE_LIMIT_CAPACITY"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			capacity = v
		}
	}
	if s := os.Getenv("RATE_LIMIT_REFILL"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			refill = v
		}
	}
	globalRateLimiter = &rateLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: capacity,
		refill:   refill,
	}
	slog.Info("rate limiter initialized", "capacity", capacity, "refill_per_second", refill)
}

func (rl *rateLimiter) getBucket(key string) *tokenBucket {
	rl.mu.RLock()
	b, ok := rl.buckets[key]
	rl.mu.RUnlock()
	if ok {
		return b
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if b, ok := rl.buckets[key]; ok {
		return b
	}
	b = &tokenBucket{
		tokens:     rl.capacity,
		capacity:   rl.capacity,
		refillRate: rl.refill,
		lastRefill: time.Now(),
	}
	rl.buckets[key] = b
	return b
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = tb.tokens + elapsed*tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	if globalRateLimiter == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if apiKey := r.Header.Get("X-Api-Key"); apiKey != "" {
			key = apiKey
		}
		b := globalRateLimiter.getBucket(key)
		if !b.allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(ErrorResponse{
				Error:     "rate limit exceeded",
				Request:   getReqID(r),
				ErrorCode: "rate_limited",
			})
			return
		}
		next(w, r)
	}
}
