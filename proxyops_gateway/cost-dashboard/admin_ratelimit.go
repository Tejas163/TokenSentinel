package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	adminRateLimit     = 100
	adminRateWindow    = 60 * time.Second
)

func initRateLimitConfig() {
	if v := os.Getenv("ADMIN_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			adminRateLimit = n
		}
	}
	if v := os.Getenv("ADMIN_RATE_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			adminRateWindow = d
		}
	}
	slog.Info("rate limit config", "limit", adminRateLimit, "window", adminRateWindow)
}

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	initRateLimitConfig()
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			if b := r.Header.Get("Authorization"); len(b) > 7 && strings.EqualFold(b[:7], "Bearer ") {
				key = b[7:]
			}
		}
		if key == "" {
			key = r.RemoteAddr
		}

		allowed, remaining, reset, err := checkRateLimit(r.Context(), key, r.URL.Path)
		if err != nil {
			slog.Error("rate limit check failed", "err", err)
			next(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", adminRateLimit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", reset))

		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded","retry_after":"` + fmt.Sprintf("%.0f", time.Until(time.Unix(reset, 0)).Seconds()) + `s"}`))
			return
		}

		next(w, r)
	}
}

func checkRateLimit(ctx context.Context, key, path string) (bool, int, int64, error) {
	now := time.Now()
	windowStart := now.Add(-adminRateWindow)
	cleanup := now.Add(-2 * adminRateWindow)

	redisKey := fmt.Sprintf("ratelimit:%s:%s", key, path)
	member := fmt.Sprintf("%s:%d", key, now.UnixNano())

	pipe := rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", cleanup.UnixNano()))
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
	pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(now.UnixNano()), Member: member})
	pipe.ZCard(ctx, redisKey)
	pipe.Expire(ctx, redisKey, 2*adminRateWindow)

	cmders, err := pipe.Exec(ctx)
	if err != nil {
		return true, adminRateLimit, now.Add(adminRateWindow).Unix(), fmt.Errorf("redis pipeline: %w", err)
	}

	count := int64(0)
	if zc := cmders[3]; zc != nil {
		if v, ok := zc.(*redis.IntCmd); ok {
			count = v.Val()
		}
	}

	remaining := adminRateLimit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	reset := now.Add(adminRateWindow).Unix()

	return count < int64(adminRateLimit), remaining, reset, nil
}
