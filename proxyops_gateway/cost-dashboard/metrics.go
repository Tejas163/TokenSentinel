package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cost_dashboard_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cost_dashboard_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	activeConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cost_dashboard_active_connections",
			Help: "Number of active SSE connections",
		},
	)
	costEntriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cost_dashboard_cost_entries_total",
			Help: "Total number of cost entries ingested",
		},
	)
	anomaliesDetected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cost_dashboard_anomalies_detected_total",
			Help: "Total number of anomalies detected",
		},
	)
	alertsFired = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cost_dashboard_alerts_fired_total",
			Help: "Total number of alerts fired",
		},
	)
	budgetAlertsFired = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cost_dashboard_budget_alerts_fired_total",
			Help: "Total number of budget alerts fired",
		},
	)
	dbQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cost_dashboard_db_query_duration_seconds",
			Help:    "Database query latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"query"},
	)
)