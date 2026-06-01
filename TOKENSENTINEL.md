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
| **3. Telemetry Monitor** | Erlang + Redis | Health heartbeat, anomaly detection, cost aggregation alerts |
| **4. Enterprise SDK** | Java/Spring Boot | Client SDK for enterprise integration, admin API, Micrometer metrics |
| **5. Prompt Optimizer** | Python (EvoluNet) | Genetic-algorithm prompt evolution, cost-aware mutation |

### Data Layer
| Component | Storage | Purpose |
|-----------|---------|---------|
| Redis | In-memory + pub/sub | Shared state: rate limits, routes, health, cost events |
| SQLite | Persistent volume | Cost dashboard historical data |
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

## Module 3: Erlang Telemetry Monitor (`proxyops_gateway/erlang-monitor/`)
- Health heartbeat to Redis (`health:erlang-monitor`, 30s TTL)
- Publishes to `health:events` channel
- Cost telemetry persistence to Redis via `SETEX`
- Budget alert polling (planned)

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
- **Health monitoring:** All services write heartbeats to Redis. Erlang monitor aggregates and alerts.
- **Security:** TLS at edge, API key auth, internal network isolation.
- **Observability:** Redis pub/sub event bus, SQLite cost history, Docker logs.

## Development Status
- [x] Module 1: Edge Proxy (Rust) — forwarding + circuit breaker working
- [x] Module 2: Orchestration Router (Go) — full dispatch + cost telemetry working
- [x] Module 3: Telemetry Monitor (Erlang) — health + cost writes working
- [x] Cost Dashboard (Go + SQLite) — persistence + API + HTML UI
- [ ] Module 4: Enterprise SDK (Spring Boot) — scaffolded, needs implementation
- [ ] Module 5: Deploy / Benchmark / E2E — scaffolded, needs implementation
- [ ] EvoluNet prompt optimizer — stubbed, needs operators
