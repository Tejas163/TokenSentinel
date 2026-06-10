# TODOS

## Phase 1: Polish & Test Coverage (high value, low effort)

- [x] **Cost dashboard structured logging** — replace log.Printf with slog
- [x] **`/metrics` endpoint** for cost-dashboard
- [x] **Go router package split** — break up 576-line main.go into types.go, middleware.go, handlers.go, routing.go
- [x] **Budget enforcement unit tests** for go-router budget check logic (enforceBudget, cheapestProvider, cheapestScore)
- [x] **Auto model selection unit tests** for selectModel() in go-router (modelTierFor, closerTier, selectModel)

## Phase 2: Production Hardening (medium effort)

- [x] **Virtual API keys — core** (M) — Redis-backed key store, validated at rust-proxy ingress, per-key rate limits, team scoping
- [x] **Virtual API keys — CRUD API** (S) — cost-dashboard endpoints for managing keys (create, revoke, list, update)
- [x] **Deploy acceleration** (S) — docker-compose.override.yml with pre-built image refs
- [x] **Performance benchmarks** (S) — k6 suite with ramp-up/soak/spike scenarios

## Phase 3: Advanced Features (larger effort)

- [ ] **OpenTelemetry trace propagation** across rust-proxy → go-router → cost-dashboard
- [ ] **K8s manifests** for cost-dashboard, mcp-gateway, and supporting services

## Phase 4: Integration

- [ ] **evolunet_slm/ integration** — 40 Python tests, no integration path into gateway
