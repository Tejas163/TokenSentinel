# ProxyOps-Gateway Architecture

## Service Communication Flow

```
                         ┌─────────────────┐
                         │   External API   │
                         │     Clients      │
                         └────────┬────────┘
                                  │ HTTPS / gRPC
                                  ▼
┌─────────────────────────────────────────────────┐
│              Rust Edge Proxy (:3000)             │
│  • TLS termination                               │
│  • API key / JWT authentication                  │
│  • Rate limiting (Redis sliding window)          │
│  • Request validation & sanitization             │
│  • Forward to Go Router                          │
└──────────────────────┬──────────────────────────┘
                       │ HTTP (internal)
                       ▼
┌─────────────────────────────────────────────────┐
│             Go Orchestration (:8080)              │
│  • Route resolution (Redis-backed rules)         │
│  • Load balancing across upstreams               │
│  • Request/response transformation               │
│  • Circuit breaker / retry logic                 │
│  • Dispatch to upstream AI providers             │
└──────────┬──────────────────────┬────────────────┘
           │                      │
           ▼                      ▼
    ┌──────────────┐    ┌──────────────────┐
    │   Redis      │    │   Upstream AI    │
    │   (:6379)    │    │   Providers      │
    │              │    │  (OpenAI, etc.)  │
    │ Shared state │    └──────────────────┘
    │ Rate limits  │
    │ Route rules  │
    │ Health keys  │
    └──────┬───────┘
            │ pub/sub
            ▼
┌─────────────────────────────────────────────────┐
│          Erlang Telemetry Monitor                 │
│  • Subscribes to Redis health keys               │
│  • Periodic health checks on all services        │
│  • Anomaly detection & alerting                  │
│  • Metrics aggregation                           │
└──────────┬──────────────────────────────────────┘
           │ subscribes to health:events (cost:*)
           ▼
┌─────────────────────────────────────────────────┐
│           Go Cost Dashboard (:3001)               │
│  • Listens for cost:* events via Redis pub/sub   │
│  • Persists cost entries to SQLite               │
│  • REST API /api/dashboard/costs, /summary       │
│  • HTML dashboard (/) — real-time cost view      │
│  • Data: model, tokens, timestamp                │
└─────────────────────────────────────────────────┘
```

## Inter-Service Contracts

### Rust → Go (internal HTTP)
- **Protocol:** HTTP/1.1 over internal Docker network
- **Headers:** `X-Request-ID`, `X-Authenticated-User`, `X-Rate-Remaining`
- **Body:** Unmodified upstream request body
- **Error:** Returns 502 if Go is unreachable

### Rust/Go → Redis
- **Protocol:** RESP over TCP
- **Namespaces:**
  - `ratelimit:{key}` — sliding window counters
  - `routes:{pattern}` — routing rule definitions
  - `health:{service}` — heartbeat timestamps

### Erlang ↔ Redis
- **Protocol:** RESP pub/sub
- **Channels:** `health:events` — service health status changes
- **Keys watched:** `health:*`, `alert:*`

### Go → Upstream Providers
- **Protocol:** HTTPS
- **Header injection:** `Authorization: Bearer <provider-key>`
- **Timeouts:** 30s per upstream request

## Data Flow per Request

1. Client sends request to Rust Edge Proxy on `:3000`
2. Rust authenticates, rate-checks via Redis, validates payload
3. Rust forwards to Go Router on `:8080` with augmented headers
4. Go resolves route from Redis rules, selects upstream
5. Go proxies to upstream AI provider, collects response
6. Go writes telemetry data to Redis
7. Response flows back: Go → Rust → Client

## Service Dependencies

| Service       | Depends On        | Depended By        |
|---------------|-------------------|--------------------|
| rust-proxy    | redis, go-router  | —                  |
| go-router     | redis             | rust-proxy         |
| redis         | —                 | rust-proxy, go-router |
