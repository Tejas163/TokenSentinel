package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client
var defaultPool *workerPool

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	authAPIKey = os.Getenv("AUTH_API_KEY")

	initSemanticCache(rdb)
	initRateLimiter()
	defaultPool = newWorkerPool(0)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", authMiddleware(healthHandler))
	mux.HandleFunc("/metrics", authMiddleware(metricsHandler))
	mux.HandleFunc("/", authMiddleware(rateLimitMiddleware(proxyHandler)))

	slog.Info("starting server", "addr", ":8080", "redis", redisAddr, "auth_enabled", authAPIKey != "")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
