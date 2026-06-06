package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

var (
	rdb               *redis.Client
	db                *sql.DB
	monitoringInterval = 5 * time.Minute
)

func lookupEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		if fallback == "" {
			log.Fatalf("required env var %q is not set", key)
		}
		log.Printf("env %q not set, using default: %s", key, fallback)
		return fallback
	}
	return v
}

func main() {
	redisAddr := lookupEnv("REDIS_ADDR", "localhost:6379")
	dsn := lookupEnv("DATABASE_URL", "postgres://localhost:5432/cost_dashboard?sslmode=disable")
	port := lookupEnv("PORT", "3002")

	if v := lookupEnv("MONITORING_INTERVAL", "5m"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			monitoringInterval = d
		}
	}

	var err error
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open postgres: %v", err)
	}
	if err = initMonitoringTables(db); err != nil {
		log.Fatalf("failed to init monitoring tables: %v", err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	initEmailConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitorSpendTrends(ctx)
	go trackSavings(ctx)
	go sendAlerts(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := rdb.Ping(r.Context()).Err(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"degraded"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Monitoring service starting on :%s...", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := <-quit
	log.Printf("received signal %v, shutting down...", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced shutdown: %v", err)
	}
	log.Println("server stopped")
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
