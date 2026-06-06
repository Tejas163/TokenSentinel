# TokenSentinel: A Distributed, Autonomous TokenOps Gateway

**Tagline:** Govern every token, every route, every cost — autonomously.

TokenSentinel is a polyglot AI gateway that provides enterprise-grade token cost governance,
intelligent provider routing, real-time cost dashboards, and autonomous prompt optimization.
Built with Rust, Go, Erlang, Python, and Java — each module speaks the language best suited
to its job.

---

## Module Architecture

| Module | Stack | Role |
|--------|-------|------|
| **1. Edge Proxy** | Rust + Redis | TLS termination, auth, rate limiting, request forwarding |
| **2. Orchestration Router** | Go + Redis | Route resolution, load balancing, circuit breaker, retry, cost telemetry |
| **3. Cost Governance** | Go + Redis + Postgres | Spend trends, savings detection, alert dispatch, budget webhooks |
| **4. Enterprise SDK** | Java/Spring Boot | Client SDK for enterprise integration, admin API, Micrometer metrics |
| **5. Prompt Optimizer** | Python (EvoluNet) | Genetic-algorithm prompt evolution, cost-aware mutation |

### Data Layer
| Component | Storage | Purpose |
|-----------|---------|---------|
| Redis | In-memory + pub/sub | Shared state: rate limits, routes, health, cost events |
| PostgreSQL | Persistent volume | Cost dashboard historical data, monitoring rules, alerts |
| Flat files (evolunet) | Disk | Prompt templates, generation scores |

---

## Module 1: Rust Edge Proxy (`proxyops_gateway/rust-proxy/`)
- TLS termination & API key / JWT auth
- Redis sliding-window rate limiting
- Circuit breaker on upstream (Go router)
- Forward to Go router with augmented headers

## Module 2: Go Orchestration Router (`proxyops_gateway/go-router/`)
- Redis-backed route resolution (`routes:{pattern}`)
- Weighted provider selection
- Retry with exponential backoff + jitter (2 retries)
- Per-provider circuit breaker (5 failures → 30s open)
- Cost telemetry writes to Redis (`sentinel:{request_id}:cost`)
- Redis pub/sub notification on cost events

## Module 3: Cost Governance + Monitoring (`proxyops_gateway/cost-dashboard/`)
- Spend trend analysis (7d vs 7d comparison, per-model alerts)
- Savings detection (>=30% cost drops logged as savings events)
- Alert dispatch via webhook (HMAC-signed), SMTP email, and SSE
- Budget threshold webhooks with 30-min cooldown
- Anomaly detection (3σ sliding window per-model)
- 3-sigma anomaly detection via SSE broadcast
- Continuous optimization engine (prescriptive reports, what-if scenarios)

## Module 4: Spring Boot Enterprise SDK (`spring-boot-sdk/`)
- Auto-configured Spring Boot Starter
- TokenCostClient — query cost data from the dashboard API
- AdminClient — manage routes, budgets, providers
- Micrometer metrics integration (meters for tokens, cost, latency)
- Rate-limited REST client with retry
- See `spring-boot-sdk/README.md` for details.

## Module 5: Deployment, Benchmark & E2E Validation (`deploy/`, `benchmark/`)
- Docker Compose (dev) + Kubernetes manifests (prod)
- k6 load testing benchmarks
- E2E docker-compose test suite
- Validation: auth flow, route resolution, cost recording, health checks

---

## Cross-Cutting Concerns
- **Cost governance:** Every request records model + token usage. Dashboard provides CFO-ready cost breakdown by model, time period, and (future) team/user.
- **Health monitoring:** All services write heartbeats to Redis. Cost dashboard aggregates and displays health status.
- **Security:** TLS at edge, API key auth, internal network isolation.
- **Observability:** Redis pub/sub event bus, SQLite cost history, Docker logs.

## Development Status
- [x] Module 1: Edge Proxy (Rust) — forwarding + circuit breaker working
- [x] Module 2: Orchestration Router (Go) — full dispatch + cost telemetry working
- [x] Module 3: Cost Governance + Monitoring — spend trends, savings detection, alert dispatch working
- [x] Cost Dashboard (Go + SQLite) — persistence + API + HTML UI
- [x] Module 4: Enterprise SDK (Spring AI) — 10 `@Tool`-annotated methods auto-registered as Spring AI `ToolCallback` beans, REST clients with retry, 21 unit tests
- [ ] Module 5: Deploy / Benchmark / E2E — scaffolded, needs implementation
- [x] EvoluNet prompt optimizer — 6 mutation operators (crossover, substitute, insert, delete, shuffle), tournament selection, elitism, fitness scoring, cost tracking, 31 tests
