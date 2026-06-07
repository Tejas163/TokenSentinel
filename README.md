# TokenSentinel

**A Distributed, Autonomous TokenOps Gateway**

[![CI](https://github.com/Tejas163/TokenSentinel/actions/workflows/ci.yml/badge.svg)](https://github.com/Tejas163/TokenSentinel/actions/workflows/ci.yml)

TokenSentinel is an enterprise-grade AI gateway that governs every token, every route, and every cost — autonomously.

## Deploy in 5 Minutes

```bash
git clone https://github.com/Tejas163/TokenSentinel.git
cd TokenSentinel
docker compose -f proxyops_gateway/docker-compose.yml up -d --build
docker compose -f proxyops_gateway/docker-compose.mcp.yml up -d --build  # optional MCP gateway
docker compose exec redis redis-cli SET "routes:/v1/chat/completions" \
  '{"pattern":"/v1/chat/completions","providers":[{"url":"https://api.openai.com/v1/chat/completions","model":"gpt-4o","weight":3,"timeout":30}]}'
open http://localhost:3001
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"Hello"}]}'
```

## Why TokenSentinel?

| Problem | Solution |
|---------|----------|
| **No cost visibility** | Every request records model + token usage. Dashboard with charts, breakdowns by model/team/period, anomaly detection, and prescriptive cost-saving assessments. |
| **Provider lock-in** | Weighted random routing with **adaptive weights** that learn from historical error rates. Circuit breaker, retry with exponential backoff + jitter, and multi-provider fallback chains. |
| **Expensive repeated prompts** | **Semantic caching** (MinHash LSH + optional vector embeddings) catches rephrased prompts — 33µs MinHash, catches similar queries at 0.85+ threshold. Embedding mode catches 3/5 rephrased variants. |
| **Budget overruns** | Team budget caps with automatic rerouting to cheapest provider when budget exceeded. Hard model context-window enforcement. |
| **Enterprise integration** | MCP protocol gateway (SSE + POST), Rust edge proxy, Spring Boot Starter with auto-configuration. |

## Architecture

```
                         ┌──────────────┐
                         │  Clients     │
                         │  (curl, SDK) │
                         └──────┬───────┘
                                │
                         ┌──────▼───────┐
                         │  Rust        │  Edge Proxy (:3000)
                         │  Edge Proxy  │  Auth, TLS, /mcp/* routing
                         └──┬───────┬───┘
                            │       │
                   ┌────────▼──┐  ┌─▼──────────┐
                   │ Go Router │  │ MCP Gateway│  MCP (:3010)
                   │ (:8080)   │  │ (:3010)    │  SSE + POST /message
                   └──┬─────┬──┘  └──┬──────────┘
                      │     │       │
              ┌───────▼──┐ │  ┌─────▼──────────┐
              │ Upstream │ │  │ Cost Dashboard │  Dashboard (:3001)
              │ AI APIs  │ │  │ (Go + Postgres)│  SSE events, charts
              └──────────┘ │  └──┬─────────────┘
                           │     │
                     ┌─────▼─────▼──────┐
                     │   Redis Stack    │
                     │  Pub/Sub + KV    │  Shared state
                     │  + RediSearch    │
                     └──────────────────┘
```

**6 services, 4 languages — each chosen for the job:**

| Service | Language | Port | Responsibility |
|---------|----------|------|---------------|
| **Edge Proxy** | Rust (Axum) | `3000` | TLS termination, API key auth, forward to router or MCP gateway |
| **Orchestration Router** | Go | `8080` | Redis route resolution, adaptive weighted selection, circuit breaker, retry, semantic cache (MinHash + Embedding), per-model limits, rate limiting, cost telemetry, fallback chains |
| **Cost Dashboard** | Go | `3001` | Real-time cost aggregation, anomaly detection (3-sigma), prescriptive engine (what-if, reports), monitoring rules + alerting (webhook/email), savings tracking, Chart.js dashboard |
| **MCP Gateway** | Rust (Axum) | `3010` | MCP protocol (SSE + POST /message), 11 cost-management tools, budget-aware access control, team data scoping |
| **Redis Stack** | — | `6379` | Routes (`routes:{path}`), rate limits (`ratelimit:{key}`), health (`health:{service}`), cost events (`sentinel:{id}:cost`), pub/sub (`health:events`), RediSearch vector index |
| **PostgreSQL** | — | `5432` | Persistent cost history, monitoring rules, alerts, assessments, reports, what-if projections |

## Environment Variables

### go-router (port 8080)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | — | Redis AUTH password |
| `AUTH_API_KEY` | (disabled) | API key for `X-Api-Key` or `Authorization: Bearer` |
| `RATE_LIMIT_CAPACITY` | `60` | Token bucket capacity (max burst) |
| `RATE_LIMIT_REFILL` | `60` | Token bucket refill rate (tokens/sec) |
| `MODEL_MAX_INPUT_TOKENS` | *(per-model)* | Global override for max input tokens |
| `MODEL_MAX_OUTPUT_TOKENS` | *(per-model)* | Global override for max output tokens |
| `SEMANTIC_CACHE_ENABLED` | `false` | Enable semantic caching |
| `SEMANTIC_CACHE_MODE` | `minhash` | `minhash` or `embedding` |
| `SEMANTIC_CACHE_THRESHOLD` | `0.85` | Similarity threshold (0.0–1.0) |
| `SEMANTIC_CACHE_TTL` | `1h` | Cache TTL (Go duration) |
| `EMBEDDING_API_URL` | `https://api.openai.com/v1/embeddings` | Embedding API endpoint |
| `EMBEDDING_MODEL` | `text-embedding-ada-002` | Embedding model name |
| `EMBEDDING_API_KEY` | — | Bearer token for embedding API |
| `EMBEDDING_DIM` | `1536` | Embedding vector dimension |
| `BUDGET_TEAM_NAME` | — | Default team for cost entries |

### cost-dashboard (port 3001)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | — | Redis AUTH password |
| `DATABASE_URL` | `postgres://localhost:5432/cost_dashboard?sslmode=disable` | PostgreSQL DSN |
| `PORT` | `3001` | HTTP listen port |
| `AUTH_API_KEY` | (disabled) | API key for dashboard auth |
| `ANOMALY_Z_SCORE` | `3.0` | Z-score threshold for anomaly detection |
| `MONITORING_INTERVAL` | `5m` | Spend trend monitoring interval |
| `ANOMALY_INTERVAL` | `5m` | Anomaly detection interval |
| `RETENTION_MAX_DAYS` | `90` | Cost entry retention in days |
| `ADMIN_RATE_LIMIT` | `100` | Max admin requests per window |
| `ADMIN_RATE_WINDOW` | `60s` | Admin rate limit window |
| `ROUTER_URL` | `http://go-router:8080` | go-router URL for health checks |
| `SMTP_HOST` | (disabled) | SMTP server for alert emails |
| `SMTP_PORT` | — | SMTP port (587 or 465) |
| `SMTP_USER` | — | SMTP AUTH username |
| `SMTP_PASS` | — | SMTP AUTH password |
| `FROM_ADDR` | `tokensentinel@<host>` | From: address for emails |
| `DIGEST_WEBHOOK_URL` | (disabled) | Slack/Teams webhook for cost digest |
| `DIGEST_SCHEDULE` | `24h` | Digest interval |
| `ENTERPRISE_EMAIL_TO` | `tejaskrshna@gmail.com` | Enterprise inquiry recipient |

### rust-proxy (port 3000)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | `redis://127.0.0.1:6379` | Redis connection URL |
| `GO_ROUTER_URL` | `http://127.0.0.1:8080` | Upstream go-router URL |
| `MCP_GATEWAY_URL` | `http://127.0.0.1:3010` | Upstream MCP gateway URL |

### mcp-gateway (port 3010)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | `redis://127.0.0.1:6379` | Redis connection URL |
| `MCP_API_KEY` | (disabled) | Comma-separated accepted MCP API keys |
| `JWT_SECRET` | (disabled) | JWT secret for token auth |
| `AGENT_TEAM_MAP` | — | `key=team,key=team` mapping for data scoping |
| `DASHBOARD_URL` | `http://localhost:3001` | Cost dashboard base URL |
| `DASHBOARD_API_KEY` | — | API key for dashboard upstream calls |

## API Reference

### Dashboard

```
GET /api/dashboard/summary?period=24h     → {"total_requests":2,"total_tokens":920,...}
GET /api/dashboard/costs?period=24h       → [{"model":"gpt-4","total_tokens":920,...}]
GET /api/dashboard/cost-timeseries?period=24h  → [{"hour":"...","model":"gpt-4","cost":0.15,"tokens":500},...]
GET /api/dashboard/anomalies?period=24h   → [{"model":"gpt-4","z_score":3.5,...}]
GET /api/dashboard/events                 → SSE stream (cost, anomaly, alert events)
GET /api/health/all                       → {redis, postgres, router health}
```

Supported periods: `1h`, `6h`, `24h`, `72h`, `168h`

### Monitoring

```
GET  /api/monitoring/rules                → List monitoring rules
POST /api/monitoring/rules                → Create rule
GET  /api/monitoring/alerts               → List alerts
POST /api/monitoring/alerts/{id}/acknowledge  → Acknowledge alert
POST /api/monitoring/alerts/{id}/dismiss  → Dismiss alert
GET  /api/monitoring/savings              → Savings events
GET  /api/monitoring/trends/{model}       → Spend trend data
```

### Admin

```
GET    /api/admin/budget-rules            → List budget rules
POST   /api/admin/budget-rules            → Create rule
DELETE /api/admin/budget-rules            → Delete rule
GET    /api/admin/teams                   → List teams
POST   /api/admin/teams                   → Create team
DELETE /api/admin/teams                   → Delete team
GET    /api/admin/pricing                 → List pricing
POST   /api/admin/seed-demo               → Seed demo data
```

### Prescriptive Engine

```
GET    /api/prescriptive/assessments             → List assessments
POST   /api/prescriptive/assessments             → Create assessment
GET    /api/prescriptive/assessments/{id}        → Get assessment
POST   /api/prescriptive/assessments/{id}/run    → Run prescriptive engine
POST   /api/prescriptive/what-if/{id}            → What-if simulation
GET    /api/prescriptive/report/{id}             → Report (HTML or JSON)
GET    /api/prescriptive/report/{id}/csv         → CSV export
GET    /api/prescriptive/report/{id}/pdf         → PDF export
```

### Playground

```
GET  /api/playground/models    → Available models with pricing
POST /api/playground/send      → Simulate a request
```

### Health

```
GET /health          → go-router: {"status":"ok"}
GET /health          → rust-proxy: {"status":"ok"}
GET /health          → cost-dashboard: {"status":"ok"}
```

## Key Features

### Adaptive Routing
`go-router/adaptive_router.go` — dynamically adjusts provider selection weights based on historical error rates. A provider with a 50% error rate gets its weight halved (floor: 5% of original). All weights recomputed on every selection. Logged at debug level when weights diverge from static config.

### Semantic Caching
`go-router/semantic_cache.go` — two-phase implementation:

- **Phase 1 (MinHash LSH)**: 64-hash FNV-1a MinHash with trigram tokenization, 16×4 LSH banding, Redis-backed. 33µs per signature. Pure Go, zero new deps, works with `redis:7-alpine`.
- **Phase 2 (Embeddings)**: Configurable embedding API (OpenAI-compatible or local LM Studio), RediSearch vector index with COSINE distance, 24h embedding cache. Switch via `SEMANTIC_CACHE_MODE=embedding`.

### Fallback Router
`go-router/fallback_router.go` — when primary provider fails, tries equivalence group members first (e.g., `gpt-4` → `claude-3-sonnet`), then cheapest remaining providers. Skips providers with open circuit breakers. Sets `X-Cost-Routing: fallback` on fallback responses.

### Production Hardening

- **Bounded goroutine pool** (`workerpool.go`): `runtime.NumCPU() × 2` workers with buffered channel. Both cost recording and cache storage use the pool. Non-blocking submit with drop-on-full.
- **Per-model context windows** (`model_limits.go`): 22 model entries with accurate `maxInputTokens`/`maxOutputTokens`. Overridable via env vars. Request rejected early with clear error message.
- **Rate limiting** (`ratelimit.go`): In-memory token bucket per API key (or per IP). Default 60 req/s. Configurable via `RATE_LIMIT_CAPACITY` and `RATE_LIMIT_REFILL`. Structured 429 response.
- **Structured errors**: All error responses include `request_id` and optional `error_code` (e.g., `"rate_limited"`).

### Monitoring Engine
`cost-dashboard/monitoring_engine.go` — background goroutines for:
- **Spend trend monitoring** (every `MONITORING_INTERVAL`): detects spikes/drops per model
- **Anomaly detection** (every `ANOMALY_INTERVAL`): 3-sigma z-score vs trailing window
- **Savings tracking**: records cost savings events from semantic cache hits
- **Alerting**: email via SMTP, webhook (Slack/Teams), SSE events
- **Data retention**: automatic pruning after `RETENTION_MAX_DAYS`
- **Cost digest**: periodic summary report via webhook

### MCP Gateway
`mcp-gateway/` — exposes 11 cost-management tools via MCP protocol:
- Cost summary, model costs, anomaly detection
- Full prescriptive assessment with what-if simulations
- Budget status, budget rules, assessment reports
- Team data scoping via `AGENT_TEAM_MAP`

## Repository Structure

```
TokenSentinel/
├── proxyops_gateway/
│   ├── rust-proxy/           # Rust edge proxy (Axum, Redis)
│   ├── go-router/            # Go orchestration router
│   ├── cost-dashboard/       # Go + Postgres dashboard + monitoring engine
│   ├── mcp-gateway/          # Rust MCP protocol gateway
│   ├── docker-compose.yml    # Main dev orchestration
│   ├── docker-compose.mcp.yml# Standalone MCP deployment
│   ├── globals.md            # Redis key conventions
│   └── AGENTS.md             # System architecture reference
├── spring-boot-sdk/          # Spring Boot Starter (Maven)
├── deploy/
│   ├── k8s/                  # Kubernetes manifests
│   └── README.md             # Deployment guide
├── benchmark/                # k6 load testing
├── sdk/python/               # Python client SDK
├── demo/                     # Demo scripts
├── TOKENSENTINEL.md          # Full module plan
└── README.md                 ← You are here
```

---

*Built with Rust, Go, Java, Python, and Postgres.*
