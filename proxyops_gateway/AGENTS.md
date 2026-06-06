# ProxyOps-Gateway — System Architecture

Multi-language AI gateway with MCP agentic gateway, orchestrated via Docker Compose.

## Services

| Service | Language | Port(s) | Role |
|---------|----------|---------|------|
| **rust-proxy** | Rust (axum) | `3000` | Ingress, auth, request forwarding |
| **go-router** | Go (net/http) | `8080` | Route resolution, load balancing, dispatch |
| **cost-dashboard** | Go (net/http) | `3001` | Cost visualization, prescriptive engine, anomaly & savings detection, alert dispatch (webhook/email/SSE) |
| **mcp-gateway** | Rust (axum) | `3010` | MCP protocol (SSE + POST /message), tool dispatch |
| **redis** | — (redis:7-alpine) | `6379` | Shared state, rate-limit, pub/sub |
| **postgres** | — (postgres:16-alpine) | `5432` | Cost dashboard data, assessments, monitoring rules |

## Data Flow

```
Client/AI Agent → rust-proxy (:3000) ─┬→ go-router (:8080) → upstream
                                       │
                                       └→ mcp-gateway (:3010) → cost-dashboard (:3001)
                                                                 ↕
                                                           redis / postgres
```

- MCP agents connect to rust-proxy `:3000` with `MCP_API_KEY`, which forwards `/mcp/*` to `mcp-gateway:3010`
- `mcp-gateway` exposes MCP HTTP transport (SSE + POST /message) with 11 tools
- Cost-aware routing uses cached model catalog from prescriptive engine, refreshed every 5min
- Team data scoping: `AGENT_TEAM_MAP` env var maps API keys to teams, forwarded as `X-Team-Name`
- Budget-aware access control: blocks tool calls for teams that exceeded budget

## MCP Tools (11)

| Tool | Description |
|------|-------------|
| `get_cost_summary` | Aggregate token cost for a period |
| `get_model_costs` | Per-model cost breakdown |
| `get_anomalies` | 3-sigma anomaly detection |
| `run_assessment` | Full cost assessment with recommendations |
| `run_whatif` | Single what-if scenario |
| `whatif_multi_scenario` | Compare multiple scenarios side-by-side |
| `whatif_volume_shift` | Model volume increase/decrease impact |
| `whatif_model_switch` | Model substitution cost impact |
| `get_budget_status` | Team budget vs usage |
| `list_budget_rules` | Budget threshold alert rules |
| `get_report` | Complete assessment report |

## Env Vars

| Var | Used By | Purpose |
|-----|---------|---------|
| `MCP_API_KEY` | mcp-gateway | Comma-separated API keys for MCP auth |
| `AGENT_TEAM_MAP` | mcp-gateway | `key=team,key=team` mapping for data scoping |
| `DASHBOARD_URL` | mcp-gateway | Cost dashboard base URL |
| `DASHBOARD_API_KEY` | mcp-gateway | API key for dashboard upstream calls |
| `REDIS_URL` / `REDIS_ADDR` | all services | Redis connection string |

## Project Structure

```
proxyops_gateway/
├── rust-proxy/           # Rust axum — ingress, auth, routing
├── go-router/            # Go — request router/load balancer
├── cost-dashboard/       # Go — cost + prescriptive engine
├── mcp-gateway/          # Rust axum — MCP protocol gateway
│   ├── src/
│   │   ├── main.rs           # Router, health, metrics
│   │   ├── mcp/              # MCP protocol layer
│   │   │   ├── transport.rs  # SSE + POST /message
│   │   │   ├── mod.rs        # Dispatch + budget check
│   │   │   ├── tools.rs      # 11 MCP tool definitions
│   │   │   ├── handlers.rs   # Tool implementations
│   │   │   ├── handlers_whatif.rs  # Multi-scenario what-if
│   │   │   ├── validation.rs # JSON Schema arg validation
│   │   │   └── budget.rs     # Budget-aware access control
│   │   ├── identity/         # Auth + data scoping
│   │   │   ├── mod.rs        # Auth middleware
│   │   │   ├── auth.rs       # API key + JWT verify
│   │   │   └── scoping.rs    # Key→team mapping
│   │   ├── prescriptive/     # Cost dashboard integration
│   │   │   ├── mod.rs
│   │   │   ├── client.rs     # REST client with retry
│   │   │   ├── catalog.rs    # Cached model catalog
│   │   │   └── router.rs     # Cost-aware routing
│   │   └── redis/            # Redis connection manager
├── docker-compose.yml
└── AGENTS.md
```

## Build Note

mcp-gateway Docker build uses `rust:1.94-slim-bookworm` (matching rust-proxy). On Windows, build via Docker (Linux containers) to avoid proc-macro build script restrictions from Windows Application Control policy.

## Tests

41 tests total — 27 unit tests + 14 integration tests.

**Unit tests (27):**
- `mcp/validation.rs` — 16 tests: schema validation, required fields, type checks, what-if args
- `mcp/mod.rs` — 2 tests: unknown method → -32601, tools/list returns ok
- `mcp/budget.rs` — 6 tests: budget block/allow, team scoping, missing field
- `identity/scoping.rs` — 3 tests: no env, after env, unknown key

**Integration tests (14)** in `tests/resilience.rs`:
- Health: returns OK, valid JSON, degraded w/o Redis
- SSE: connect returns streaming, double connect, POST rejected
- Message: missing session, invalid JSON, nonexistent session, no-auth, tools/list
- Routing: unknown route 404, POST to SSE 405, GET to message 405

Run via: `docker run --rm -v "${PWD}:/app" -v cargo-registry:/root/.cargo -w /app rust:1.94-slim-bookworm sh -c 'apt-get update -qq && apt-get install -y -qq pkg-config libssl-dev > /dev/null 2>&1 && cargo test'`
