# TokenSentinel

**A Distributed, Autonomous TokenOps Gateway**

[![CI](https://github.com/Tejas163/TokenSentinel/actions/workflows/ci.yml/badge.svg)](https://github.com/Tejas163/TokenSentinel/actions/workflows/ci.yml)

TokenSentinel is an enterprise-grade AI gateway that governs every token, every route, and every cost — autonomously. Built with a polyglot architecture where each service speaks the language best suited to its job.

```
┌──────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  Rust        │───▶│  Go              │───▶│  Upstream        │
│  Edge Proxy  │    │  Router          │    │  AI Providers    │
│  (:3000)     │    │  (:8080)         │    │  (OpenAI, etc.)  │
└──────┬───────┘    └────────┬─────────┘    └──────────────────┘
       │                     │
       │              ┌──────▼──────────┐    ┌──────────────────┐
       │              │  Redis (:6379)  │◀───│  Erlang Monitor  │
       │              │  Pub/Sub + KV   │    │  Health + Cost   │
       │              └──────┬──────────┘    └──────────────────┘
       │                     │ cost:* events
       │              ┌──────▼──────────┐    ┌──────────────────┐
       └──────────────│  Cost Dashboard │    │  Spring Boot SDK │
                      │  Go + SQLite    │    │  Java Client     │
                      │  (:3001)        │    │  (Maven)         │
                      └─────────────────┘    └──────────────────┘
```

## Why TokenSentinel?

| Problem | TokenSentinel Solution |
|---------|----------------------|
| **No cost visibility** | Every request records model + token usage. CFO-ready dashboard with breakdowns by model and time period. |
| **Provider lock-in** | Weighted random routing across providers. Add/remove upstreams without code changes. |
| **Expensive failures** | Circuit breaker (5 failures → 30s cooldown), retry with exponential backoff + jitter. |
| **Enterprise integration** | Spring Boot Starter with auto-configuration, Micrometer metrics, typed Java clients. |

## Architecture

**5 modules, 5 languages — each chosen for the job:**

| Module | Language | Responsibility |
|--------|----------|---------------|
| **Edge Proxy** | Rust | TLS termination, API key auth, Redis sliding-window rate limiting, forward to router |
| **Orchestration Router** | Go | Redis-backed route resolution, weighted provider selection, circuit breaker, retry, cost telemetry |
| **Telemetry Monitor** | Erlang | Health heartbeat (30s TTL), cost persistence to Redis, pub/sub event bus |
| **Enterprise SDK** | Java/Spring Boot | Auto-configured clients (TokenCostClient, AdminClient), Micrometer metrics |
| **Cost Dashboard** | Go + SQLite | Real-time cost aggregation API, HTML dashboard with period filtering |

**Data layer:**
- **Redis** — Shared state: rate limits (`ratelimit:{key}`), routes (`routes:{pattern}`), health (`health:{service}`), cost events (`sentinel:{request_id}:cost`)
- **SQLite** — Persistent cost history for the dashboard
- **Pub/sub** — `health:events` channel for real-time cost ingestion

## Quick Start

```bash
# Start all services
docker compose -f proxyops_gateway/docker-compose.yml up -d --build

# Verify everything is running
docker compose -f proxyops_gateway/docker-compose.yml ps

# Run end-to-end validation
./deploy/run-e2e.sh
```

**Services:**

| Service | Port | URL |
|---------|------|-----|
| Rust Proxy | 3000 | `http://localhost:3000/health` |
| Go Router | 8080 | `http://localhost:8080/health` |
| Cost Dashboard | 3001 | `http://localhost:3001/` |
| Redis | 6379 | `redis://localhost:6379` |

## E2E Verification

All 5 services verified live with real data flow:

```
=== SERVICE HEALTH ===
Redis:                          PONG
Go Router (:8080):              {"status":"ok"}
Rust Proxy (:3000):             {"status":"ok"}
Erlang Monitor Heartbeat:       1780299326 (TTL: 15s)
Cost Dashboard (:3001):         HTTP 200

=== COST DATA FLOW ===
Inject → Redis SETEX sentinel:{id}:cost {"model":"gpt-4",...}
     → PUBLISH health:events cost:{id}
     → Cost Dashboard consumes → SQLite INSERT
     → API returns: {"total_requests":2, "total_tokens":920}

=== ROUTE CONFIGURATION ===
Redis SET routes:/v1/chat/completions → {"providers":[...]}
```

## API Reference

### Cost Dashboard

```
GET /api/dashboard/summary?period=24h
  → {"total_requests": 2, "total_tokens": 920, ...}

GET /api/dashboard/costs?period=24h
  → [{"model": "gpt-4", "total_tokens": 920, ...}]

GET /                           → HTML dashboard
```

Supported periods: `1h`, `6h`, `24h`, `72h`, `168h`

### Health

```
GET /health (Go Router :8080)
  → {"status": "ok"}

GET /health (Rust Proxy :3000)
  → {"status": "ok"}
```

## Deployment

### Docker Compose (dev)
```bash
docker compose -f proxyops_gateway/docker-compose.yml up -d
```

### Kubernetes (prod)
```bash
kubectl apply -f deploy/k8s/
```

### Benchmarks
```bash
cd benchmark
k6 run k6-load-test.js
```

### Spring Boot SDK
```xml
<dependency>
    <groupId>com.tokensentinel</groupId>
    <artifactId>token-sentinel-sdk</artifactId>
    <version>0.1.0</version>
</dependency>
```

```yaml
tokensentinel:
  gateway-url: http://localhost:8080
  dashboard-url: http://localhost:3001
  api-key: ${TOKENSENTINEL_API_KEY}
```

## Repository Structure

```
TokenSentinel/
├── proxyops_gateway/
│   ├── rust-proxy/          # Rust edge proxy (Axum, Redis)
│   ├── go-router/           # Go orchestration router
│   ├── erlang-monitor/      # Erlang OTP telemetry monitor
│   ├── cost-dashboard/      # Go + SQLite dashboard (embedded HTML)
│   ├── docker-compose.yml   # Dev orchestration
│   ├── globals.md           # Redis key conventions
│   └── proxyops-docs/       # Architecture docs
├── spring-boot-sdk/         # Spring Boot Starter (Maven)
├── deploy/
│   ├── k8s/                 # Kubernetes manifests
│   ├── run-e2e.sh           # E2E validation script
│   └── README.md            # Deployment guide
├── benchmark/               # k6 load testing
├── evolunet_slm/            # Python prompt optimizer (stub)
├── TOKENSENTINEL.md         # Full module plan
├── README.md                ← You are here
└── .github/workflows/       # CI pipeline
```

---

*Built with Rust, Go, Erlang, Java, Python, and Redis.*
