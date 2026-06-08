# TODOS

## Phase 1: Polish & Test Coverage (high value, low effort)

- [ ] **Cost dashboard structured logging** — replace log.Printf with slog
- [ ] **`/metrics` endpoint** for cost-dashboard
- [x] **Go router package split** — break up 576-line main.go into types.go, middleware.go, handlers.go, routing.go
- [x] **Budget enforcement unit tests** for go-router budget check logic (enforceBudget, cheapestProvider, cheapestScore)
- [x] **Auto model selection unit tests** for selectModel() in go-router (modelTierFor, closerTier, selectModel)

## Phase 2: Production Hardening (medium effort)

- [ ] **Virtual API keys** (M) — Redis-backed key store, per-service/per-key auth, per-key budgets + rate limits
- [ ] **Deploy acceleration** (S) — pre-built Docker images, docker-compose.override.yml
- [ ] **Performance benchmarks** (S) — k6 suite + published results (RPS, p50/p95/p99)

## Phase 3: Advanced Features (larger effort)

- [ ] **OpenTelemetry trace propagation** across rust-proxy → go-router → cost-dashboard
- [ ] **K8s manifests** for cost-dashboard, mcp-gateway, and supporting services

## Phase 4: Integration

- [ ] **evolunet_slm/ integration** — 40 Python tests, no integration path into gateway
