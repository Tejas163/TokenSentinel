# MCP + A2A Agentic Gateway — Engineering Blueprint

**Date:** 2026-06-04
**Status:** Draft / In Review
**Vision:** TokenSentinel Phase 2 — Full agentic AI infrastructure platform

---

## 1. Why a New Rust Microservice?

| Option | Verdict |
|--------|---------|
| Extend Go router | Go handles request dispatch well, but MCP/A2A protocol parsing needs zero-copy, low-latency I/O at the edge. Rust is the right tool. |
| Python service | Python is already used for EvoluNet (offline optimization). MCP/A2A are latency-sensitive protocol handlers — Python adds GC pauses at high throughput. |
| **New Rust microservice** | **✅ Chosen.** Sits alongside the existing `rust-proxy`. Shares Redis connection, same axum/tokio stack. Clean separation of concerns. |

## 2. Architecture

**NOTE:** MCP gateway routes through rust-proxy for Phase 1 auth (decided in CEO review — outside voice finding). Agents connect to `rust-proxy:3000` which forwards MCP traffic to `mcp-gateway:3010`. This ensures auth from day 1 without waiting for Phase 2 identity system. A2A protocol removed from scope (Google's A2A draft has negligible ecosystem adoption — revisit when an integration partner exists).

```
┌──────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  AI Agents   │───▶│  rust-proxy      │───▶│  mcp-gateway     │───▶ MCP Servers
│  (Claude,    │    │  (:3000)         │    │  (:3010)         │
│  LangChain,  │    │  auth + forward  │    │  MCP only        │
│  Custom)     │    └──────────────────┘    └────────┬─────────┘
└──────────────┘                                     │
                                                     │ reads
                                              ┌──────▼──────────┐
                                              │  Redis (:6379)  │
                                              │  Pub/Sub + KV   │
                                              └──────┬──────────┘
                                                     │ cost:* events
                                              ┌──────▼──────────┐
                                              │  Cost Dashboard │───▶ Prescriptive
                                              │  Go + SQLite    │     Engine
                                              │  (:3001)        │
                                              └─────────────────┘
```

### Port allocation
- `mcp-gateway` listens on **`:3010`** — MCP protocol endpoint (internal only, behind rust-proxy)
- `mcp-gateway` listens on **`:3012`** — internal health + admin API
- Agents connect to **rust-proxy `:3000`**, which authenticates and forwards to `mcp-gateway:3010`
- A2A protocol dropped from scope — revisit when an agent integration partner exists

## 3. Components

### 3.1 MCP Server (`mcp/`)
Implements the [Model Context Protocol](https://modelcontextprotocol.io) specification:
- **Tool discovery** — exposes TokenSentinel capabilities as MCP tools via `tools/list`
- **Tool invocation** — handles `tools/call` requests, dispatches to internal handlers or proxied MCP servers
- **Resource access** — serves TokenSentinel data (cost summaries, anomalies) as MCP resources
- **Prompt templates** — serves predefined prompt templates for common FinOps queries

### 3.2 Agent Identity, Auth & Data Scoping (`identity/`)
- **Agent API keys** — scoped per-agent, with read/write/admin roles
- **Phase 1 auth** — rust-proxy authenticates all traffic (MCP_API_KEY env var), forwards to mcp-gateway
- **Phase 2+** — JWK/JWT token issuance + verification for per-agent identity
- **Data scoping** — agent identity (from API key) passed as X-Team-Name header to cost dashboard. Dashboard already supports team-based cost filtering. Added Phase 1.
- **Audit trail** — every tool call, resource access logged
- **Rate limits** — per-agent token burn rate (RPM, TPM) via Redis sliding window

### 3.4 Prescriptive Engine Adapter (`prescriptive/`)  *(Phase 3)*
- **REST client** — calls cost-dashboard prescriptive API for routing intelligence
- **Cached model catalog** — periodic refresh from prescriptive engine's model catalog and equivalence groups
- **Routing decisions** — map MCP tool requests to optimal model/provider based on cost, latency, budget

### 3.5 Redis Integration (`redis/`)
Shares the existing Redis instance:
- Reads rate limit counters (`ratelimit:{key}`)
- Reads route configuration (`routes:{pattern}`)
- Reads health state (`health:{service}`)
- Publishes cost events (`sentinel:{request_id}:cost`) for dashboard consumption
- Agent session state (`agent:{id}:session`)

## 4. MCP Tool Surface (Phase 1)

The following TokenSentinel capabilities are exposed as MCP tools:

| MCP Tool | Backing Service | Description |
|----------|----------------|-------------|
| `get_cost_summary` | Cost Dashboard API | Aggregate cost for period |
| `get_model_costs` | Cost Dashboard API | Per-model breakdown |
| `get_anomalies` | Cost Dashboard API | 3-sigma anomaly detection |
| `run_assessment` | Prescriptive Engine | Full cost assessment with recommendations |
| `run_whatif` | Prescriptive Engine | What-if scenario modeling |
| `get_budget_status` | Cost Dashboard API | Team budget vs usage |
| `list_tools` | Proxy MCP servers | Forward to registered MCP servers |
| `call_tool` | Proxy MCP servers | Invoke tools on remote MCP servers |

## 5. API Surface

### MCP Endpoint — MCP HTTP Transport (SSE + POST /message)

Per MCP spec for HTTP: server sends events over SSE, client sends requests via `POST /message`.

```
Client connects → GET /mcp/v1/sse  (opens SSE stream, receives endpoint URL)
Client sends    → POST /mcp/v1/message (JSON-RPC body, tools/list, tools/call)
Server responds → SSE event on the open stream
```

Example tool call:
```json
// Request (POST /mcp/v1/message)
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"run_assessment","arguments":{"assessment_id":42}}}

// Response (SSE event)
event: message
data: {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"{...}"}]}}
```

### Scaling — Single Instance Phase 1
- Single `mcp-gateway` replica for Phase 1
- Horizontal scaling (K8s multi-replica with Redis-backed session affinity) deferred to Phase 4
- Health endpoint ready for load balancer integration from day 1

### Admin API (`GET/POST /admin/v1`)
- `GET /admin/v1/agents` — list registered agents
- `POST /admin/v1/agents` — register new agent API key
- `GET /admin/v1/audit` — query audit log
- `GET /admin/v1/health` — health check
- `GET /admin/v1/metrics` — Prometheus metrics

## 6. Implementation Phases  *(rebaselined to 10 weeks after CEO review)*

### Phase 1: Foundation (Weeks 1-3)
- [x] Scaffold Rust crate — done (cf81d18)
- [ ] Implement MCP HTTP transport: SSE endpoint + POST /message (`mcp/transport.rs`)
- [ ] Rust-proxy routing: add MCP route to rust-proxy config, forward to mcp-gateway:3010
- [ ] Static API key auth on rust-proxy for MCP traffic
- [ ] Data scoping: pass X-Team-Name from agent API key to upstream calls (`identity/scoping.rs`)
- [ ] Implement `tools/list` returning 8 TokenSentinel tools (`mcp/tools.rs`)
- [ ] Implement `tools/call` for cost dashboard operations (`mcp/handlers.rs`)
- [ ] REST client to cost-dashboard API with retry + backoff (`prescriptive/client.rs`)
- [ ] Redis connection pool (dedicated ConnectionManager) (`redis/mod.rs`)
- [ ] SSE session lifecycle: 60s idle timeout + cleanup
- [ ] Tool argument validation against input_schema
- [ ] Health + metrics endpoint
- [ ] Dockerfile + docker-compose integration (route MCP through rust-proxy)

### Phase 2: Agent Identity (Weeks 4-7)
- [ ] JWK/JWT token issuance + verification (`identity/jwt.rs`)
- [ ] Per-agent API key scoping with team/cost-center mapping
- [ ] Audit log to Redis stream (`identity/audit.rs`)
- [ ] Per-agent rate limiting (RPM, TPM via Redis)
- [ ] Data scoping enforcement: verify agent's team filter matches their key scope
- [ ] Admin API: CRUD for agent keys, audit log query

### Phase 3: Prescriptive Engine Integration (Weeks 8-10)
- [ ] Cached model catalog from prescriptive engine (`prescriptive/catalog.rs`)
- [ ] Cost-aware MCP tool routing (`prescriptive/router.rs`)
- [ ] What-if scenario via MCP (`mcp/handlers_whatif.rs`)
- [ ] Budget-aware tool access control
- [ ] Resilience tests: Redis failover, upstream 500/429, SSE reconnect

### Phase 4: Full Platform (Months 5-6, deferred start)
- [ ] MCP resource endpoints (serve cost data as resources)
- [ ] MCP prompt templates (pre-made FinOps queries)
- [ ] Remote MCP server proxying
- [ ] Request tracing (X-Request-Id propagation)
- [ ] Kubernetes manifests + deploy sequencing
- [ ] Horizontal scaling (multi-replica)
- [ ] End-to-end tests

## 7. Integration with Existing Services

### Cost Dashboard
- Reads prescriptive engine data via REST (`GET /api/prescriptive/...`)
- Posts cost events to Redis for dashboard visibility

### rust-proxy (existing)
- MCP gateway sits alongside, not behind, the rust-proxy
- rust-proxy handles external user traffic; mcp-gateway handles agent traffic
- Both share Redis for rate limit data

### Redis
- New key namespacing: `agent:{agent_id}:session`, `agent:{agent_id}:ratelimit`
- New pub/sub channel: `agent:events` for agent-to-agent messaging

## 8. Dependencies (Cargo.toml)

```toml
[package]
name = "mcp-gateway"
version = "0.1.0"
edition = "2024"

[dependencies]
axum = "0.8"
tokio = { version = "1", features = ["full"] }
redis = { version = "0.28", features = ["tokio-comp", "connection-manager"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
reqwest = { version = "0.12", features = ["json"] }
tower = "0.5"
tower-http = { version = "0.6", features = ["cors", "trace"] }
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["env-filter"] }
uuid = { version = "1", features = ["v4"] }
jsonwebtoken = "9"
jsonrpsee = { version = "0.24", features = ["server"] }  # JSON-RPC for MCP
```

## 9. Testing Strategy

| Layer | Approach |
|-------|----------|
| Unit tests | `#[cfg(test)]` modules per component (transport, tools, identity, routing) |
| Integration | Docker Compose with Redis + cost-dashboard mock |
| MCP compliance | `mcp-spec-test` suite for protocol correctness |
| A2A e2e | Two mcp-gateway instances messaging each other |
| Load test | k6 scenario: 100 concurrent agents, mixed tool calls |

## 10. Error & Rescue Registry

Every codepath that can fail, its exception class, rescue status, and user impact:

| CODEPATH | WHAT CAN GO WRONG | EXCEPTION CLASS |
|---|---|---|
| SSE connection | Client disconnects mid-stream | ConnectionReset |
| | Client never reads from SSE stream | SlowClient |
| | SSE channel buffer full | BackpressureOverflow |
| MCP POST /message | Malformed JSON body | JsonParseError |
| | Unknown method name | MethodNotFound |
| | Missing required params | InvalidParams |
| | Internal handler panics | Panic |
| Cost dashboard REST | Dashboard pod unreachable | ConnectionRefused |
| | Dashboard returns 500 | UpstreamError |
| | Dashboard returns 429 rate limit | RateLimited |
| | Dashboard response malformed | DeserializeError |
| | Dashboard timeout (>5s) | TimeoutError |
| Redis | Redis unreachable | ConnectionRefused |
| | Redis auth failure | AuthError |
| | Key not found | NilKey |
| | Connection pool exhausted | PoolExhausted |
| Agent auth | Missing API key header | MissingAuth |
| | Invalid API key | InvalidAuth |
| | Expired/ malformed JWT | JwtError |
| A2A registration | Agent already registered | DuplicateAgent |
| | Registration payload invalid | InvalidRegistration |
| A2A task delegation | Target agent unreachable | AgentUnreachable |
| | Task times out (>30s) | TaskTimeout |
| | Target agent returns error | AgentError |
| Prescriptive engine | Engine returns invalid recommendation | InvalidRecommendation |
| | Engine call times out | PrescriptiveTimeout |
| | Model catalog stale (cache miss) | StaleCatalog |

| EXCEPTION CLASS | RESCUED? | RESCUE ACTION | USER SEES |
|---|---|---|---|
| ConnectionReset | Y | Clean up SSE channel, log warn | Disconnected (reconnect expected) |
| SlowClient | Y | Drop connection after 60s idle | Disconnected |
| BackpressureOverflow | Y | Drop oldest event, log error | Missing event (gap in stream) |
| JsonParseError | Y | Return MCP error -32700 | "Parse error" JSON-RPC response |
| MethodNotFound | Y | Return MCP error -32601 | "Method not found" JSON-RPC response |
| InvalidParams | Y | Return MCP error -32602 | "Invalid params" JSON-RPC response |
| Panic | Y | axum catch-panic middleware, log stack trace | 500 Internal Server Error |
| ConnectionRefused | N ← GAP | — | 500 ← BAD (silent upstream failure) |
| UpstreamError | Y | Log upstream response body, return 502 | "Upstream service unavailable" |
| RateLimited | N ← GAP | — | 429 passed through ← acceptable? |
| DeserializeError | Y | Log raw response, return error | "Unexpected response from upstream" |
| TimeoutError | Y | Return 504 with timeout message | "Request timed out" |
| ConnectionRefused (Redis) | Y | Health endpoint shows degraded | Health check reports degraded |
| AuthError (Redis) | Y | Log, crash on startup (misconfig) | Service won't start (good) |
| NilKey | Y | Return empty/default, no error | Empty result, no error |
| PoolExhausted | Y | Backpressure: queue request, wait for connection | Increased latency, no failure |
| MissingAuth | Y | Return 401 "Missing API key" | 401 Unauthorized |
| InvalidAuth | Y | Return 401 "Invalid API key" | 401 Unauthorized |
| JwtError | Y | Return 401 "Invalid token" | 401 Unauthorized |
| DuplicateAgent | Y | Return 409 "Agent already registered" | 409 Conflict |
| InvalidRegistration | Y | Return 400 with validation errors | 400 Bad Request |
| AgentUnreachable | Y | Return task status "failed" with error | Task shows as failed |
| TaskTimeout | Y | Return task status "timed_out" | Task shows as timed out |
| AgentError | Y | Return task status "failed" with agent's error | Task shows failed with error detail |
| InvalidRecommendation | Y | Log, fall back to default routing | Uses default routing (no cost optimization) |
| PrescriptiveTimeout | Y | Log, fall back to default routing | Uses default routing (no cost optimization) |
| StaleCatalog | Y | Log, use cached catalog, trigger refresh | No visible impact |

### CRITICAL GAPS

| GAP | FIX | PRIORITY | STATUS |
|---|---|---|---|
| ConnectionRefused (upstream) not rescued — returns 500 with no detail | Add retry with backoff (2x, 50ms/100ms), then return 502 with "Cost dashboard unavailable" | P1 | ✅ RESOLVED — add retry wrapper to REST client |
| RateLimited from upstream passed to caller without backoff | Catch 429, apply exponential backoff + jitter, retry once | P2 | Pending |
| SSE backpressure default (drop oldest) may lose critical cost events | Consider prioritized channel: keep latest event per type, drop oldest non-critical | P2 | Pending |

## 11. Security: Tool Argument Validation

Every MCP tool argument validated server-side against its declared `input_schema` before forwarding:
- `get_cost_summary`, `get_model_costs`, `get_anomalies`: `period` must match ^(1h|6h|24h|72h|168h)$
- `run_assessment`, `get_report`: `assessment_id` must be positive integer
- `get_budget_status`: `team` sanitized (alphanumeric + hyphens only)
- `run_whatif`: `adjustments` object keys validated against allowed set
- Rejects unknown keys — no silent passthrough

## 12. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| MCP spec still evolving | Pin to a stable spec revision. Abstract transport layer for future protocol changes. |
| JSON-RPC performance | Axum extractors + streaming bodies for large tool results. Pooled Redis connections. |
| Agent auth complexity | Start simple (static API keys → JWK rotation). Audit trail first, advanced auth later. |
| Prescriptive engine coupling | Store interface pattern (same as prescriptive engine). Mock store for unit tests. |
| Port conflicts / docker-compose | Assign :3010-3012. Verify no overlap with :3000 (rust-proxy), :8080 (go-router), :3001 (dashboard). |
| A2A spec volatility | A2ATransport adapter trait (GoogleA2A implementation). Swap implementation if ecosystem converges on different protocol. |

## 13. Success Criteria

- [ ] MCP `tools/list` returns 8+ TokenSentinel tools
- [ ] MCP `tools/call` dispatches to cost-dashboard and prescriptive engine
- [ ] Agent identity auth works (API key → JWT → verified on every call)
- [ ] A2A agent registration + task delegation round-trip
- [ ] Audit log captures every tool invocation
- [ ] Integrates with existing docker-compose without port conflicts
- [ ] <5ms p99 overhead on MCP tool dispatch
- [ ] 100+ concurrent agents handled without degradation

---

## CEO Review Decisions

Decisions made during the /plan-ceo-review sections 1-10:

| # | Issue | Decision | Source | IMPACT |
|---|---|---|---|---|---|
| 1 | MCP transport | Proper MCP HTTP transport (SSE + POST /message) | Internal | Compatible with all MCP client SDKs |
| 2 | Single point of failure | Single instance Phase 1. HA deferred to Phase 4. | Internal | Acceptable for MVP |
| 3 | A2A protocol | **DROPPED** — ship MCP only. Keep A2ATransport trait as hook. Revisit with integration partner. | Outside voice | Saves 4+ weeks of work on draft spec with no adoption |
| 4 | Phase 1 auth | Route MCP through rust-proxy (existing auth), not direct port exposure | Outside voice | Auth from day 1, no 2-week gap |
| 5 | Data isolation | Add per-agent data scoping (X-Team-Name) to Phase 1 | Outside voice | Agent A cannot see Agent B's costs |
| 6 | Upstream retry | Retry with backoff (2x, 50ms/100ms) + clean 502 | Internal | P1 gap resolved |
| 7 | Tool arg injection | Server-side validation against input_schema | Internal | Rejects path traversal, unknown keys |
| 8 | SSE lifecycle | 60s idle timeout + session-id routing + idempotency keys | Internal | Prevents zombie connections |
| 9 | Cargo dep | Drop jsonrpsee — axum SSE is sufficient | Internal | Leaner deps |
| 10 | Resilience tests | Add to Phase 3 | Internal | Catches production failures |
| 11 | Redis pool sizing | Dedicated ConnectionManager for agent traffic | Internal | No contention with rust-proxy/go-router |
| 12 | Request tracing | Defer to Phase 4 | Internal | Acceptable for dev |
| 13 | Deploy sequencing | Defer to Phase 4 | Internal | Acceptable |
| 14 | A2A adapter trait | Not needed (A2A dropped). Remove from scope. | Outside voice | Simplifies architecture |
| 15 | Timeline rebaseline | 10 weeks (was 6 months to platform beta) | Outside voice | Realistic for single engineer |
| 16 | Prescriptive adapter | Keep in Phase 3 (low risk, existing engine) | User choice | Matches original plan |

### What already exists
- **Rust axum/tokio stack** in `rust-proxy` — same deps, build patterns, Dockerfile style
- **Redis** — shared instance already configured, key conventions documented
- **Cost dashboard API** — REST endpoints for summary, costs, anomalies, budget rules
- **Prescriptive engine** — assessment, what-if, report, recommendation APIs (just landed)
- **Spring Boot SDK** — `TokenSentinelAiTools` with 10 `@Tool` methods (models for tool surface)

### NOT in scope (this phase)
- **A2A protocol** — dropped entirely. Revisit when an agent integration partner exists.
- **HA / multi-replica** — deferred to Phase 4
- **Full request tracing** — deferred to Phase 4
- **Kubernetes operator** — deferred to Phase 4
- **WebSocket transport for MCP** — SSE + POST /message is the MCP HTTP standard
- **Agent marketplace / discovery service** — agents register manually for now
- **Prompt caching integration** — not needed until EvoluNet integration
- **Direct MCP port exposure** — MCP traffic routes through rust-proxy

### Dream state delta

```
  INITIAL PLAN                      AFTER CEO REVIEW                 12-MONTH IDEAL
  ──────────────                    ───────────────                 ─────────────────
  MCP + A2A + identity       →      MCP only — no A2A.        →    MCP gateway with
  + cost-aware routing              10-week timeline.               agent identity,
  in 6 weeks.                       Traffic through rust-proxy.      data scoping,
  Agents on direct port.             Data scoping from day 1.        cost-aware routing,
  No data isolation.                 Auth from day 1.                production HA,
                                                                     and one real
                                                                     agent integration.
```

### Review completion

```
  +====================================================================+
  |            MEGA PLAN REVIEW — COMPLETION SUMMARY                   |
  +====================================================================+
  | Mode selected        | SCOPE EXPANSION                             |
  | System Audit         | 12 modified files in-flight (landed)        |
  | Step 0               | Expansion mode + MCP+A2A chosen             |
  | Sections 1-10        | 16 decisions (12 review + 4 outside voice)  |
  | Section 11 (Design)  | SKIPPED (no UI scope)                       |
  | Outside voice        | CLAUDE SUBAGENT — 10 findings, 4 accepted   |
  +--------------------------------------------------------------------+
  | Scope cuts           | A2A dropped (outside voice)                 |
  |                     | Timeline: 6 weeks → 10 weeks                 |
  |                     | Direct port access removed                   |
  | Scope adds           | Data isolation (Phase 1)                    |
  |                     | Auth through rust-proxy (Phase 1)            |
  | Scope kept           | Prescriptive adapter (Phase 3)              |
  | NOT in scope         | 8 items                                     |
  | What already exists  | 5 items                                     |
  | Error/rescue registry| 27 methods, 0 remaining CRITICAL GAPS       |
  | Diagrams produced    | Architecture, data flow, error flow         |
  | Lake Score           | 14/16 recommendations chose complete option  |
  +====================================================================+
```
