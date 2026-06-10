# TokenSentinel — Onboarding Guide

TokenSentinel is an open-source AI/LLM cost governance gateway. It routes requests, tracks every token and cost in real time, enforces per-team budgets, detects anomalies, and provides cost-saving recommendations.

---

## Quick Start

### Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Windows, macOS, or Linux)
- Git

### 1. Clone & Start

```powershell
git clone https://github.com/Tejas163/TokenSentinel.git
cd TokenSentinel
docker compose -f proxyops_gateway/docker-compose.yml up -d --build
```

This starts 6 services (first build takes a few minutes for Rust and Go compilation):

| Service | Port | Role |
|---------|------|------|
| rust-proxy | `3000` | Edge proxy — auth, request forwarding |
| go-router | `8080` | Request routing, load balancing, semantic cache |
| cost-dashboard | `3001` | Web UI, cost analytics, anomaly detection |
| mcp-gateway | `3010` | MCP protocol for AI agent tool dispatch |
| Redis | `6379` | Rate limits, routes, pub/sub, cache |
| PostgreSQL | `5432` | Persistent cost data, assessments, alerts |

### 2. Log In to the Dashboard

1. Open **http://localhost:3001** in your browser
2. Click **Launch Dashboard** (or go directly to `http://localhost:3001/dashboard`)
3. You'll be redirected to the **login page**
4. Enter the dev API key: **`dev-key-123`**
5. Click **Sign In**

### 3. Seed Demo Data

Once logged in, the dashboard will be empty. Click **Seed Demo Data** to populate it with 7 days of realistic cost data across 7 models and 4 teams — or send real traffic through the gateway.

---

## Managing API Keys

### Seeded Dev Key

The key `dev-key-123` is automatically created on startup with these properties:

| Property | Value |
|----------|-------|
| name | Dev Key |
| team | engineering |
| status | active |
| monthly budget | $5,000 |
| rate limit | 100 req/s |
| allowed models | all (`*`) |
| allowed services | proxy, mcp, dashboard |

### Create a New Key (via API)

```powershell
curl.exe -X POST http://localhost:3001/api/admin/keys `
  -H "Content-Type: application/json" `
  -H "X-Api-Key: dev-key-123" `
  -d '{"name":"My Key","team":"engineering","budget_monthly_cents":100000,"rate_limit_rps":50,"allowed_services":["proxy","mcp","dashboard"]}'
```

Returns the new key (auto-generated with `sk_` prefix).

### List All Keys

```powershell
curl.exe http://localhost:3001/api/admin/keys -H "X-Api-Key: dev-key-123"
```

### Get a Specific Key

```powershell
curl.exe "http://localhost:3001/api/admin/keys?key=dev-key-123" -H "X-Api-Key: dev-key-123"
```

### Update a Key

```powershell
curl.exe -X PUT "http://localhost:3001/api/admin/keys?key=dev-key-123" `
  -H "Content-Type: application/json" `
  -H "X-Api-Key: dev-key-123" `
  -d '{"status":"inactive"}'
```

### Delete a Key

```powershell
curl.exe -X DELETE "http://localhost:3001/api/admin/keys?key=dev-key-123" -H "X-Api-Key: dev-key-123"
```

> All API key endpoints require authentication via `X-Api-Key` or `Authorization: Bearer` header.

---

## Using the Dashboard

Navigate to **http://localhost:3001/dashboard** (after logging in).

### Dashboard Features

| Feature | Description |
|---------|-------------|
| **Summary Cards** | Requests, total tokens, input/output tokens, models used, avg tokens/req |
| **Period Selector** | 1H / 6H / 24H / 3D / 7D — click to reload data for the selected window |
| **Cost Over Time Chart** | Stacked line chart of costs by model across hourly buckets |
| **Cost by Model Chart** | Doughnut chart showing cost distribution across models |
| **Cost by Model Table** | Per-model breakdown with request count and token volumes |
| **Live Updates** | SSE badge turns green when connected — new costs appear in real time |
| **Anomaly Alerts** | Red banner when 3-sigma anomalies are detected |
| **Alert Toasts** | Pop-up notifications for spend/savings events |
| **Active Alert Banner** | Shows unacknowledged alerts with acknowledge/dismiss buttons |
| **Playground** | Toggle panel to simulate an LLM request and watch it appear on the dashboard |
| **Onboarding Wizard** | Appears on first empty dashboard — guides through seeding demo data |

### Playground

The playground lets you send test LLM requests to see costs appear in real time:

1. Click **Playground** (shield icon in the top bar)
2. Select a model, enter input/output token counts, pick a team name
3. Click **Send Test Request**
4. The cost entry appears immediately in the dashboard table and charts

---

## Authentication Flow

TokenSentinel uses **virtual API keys** stored in Redis. Here's how auth works end-to-end:

```
Browser -> /login -> POST api_key -> Redis validates -> cookie set
Browser -> /dashboard -> cookie read -> API key embedded in page
Browser JS -> API calls -> X-Api-Key header -> data returned
```

1. All external requests go through `rust-proxy` (port 3000)
2. `rust-proxy` checks the key against Redis via `HGETALL apikey:{key}`
3. If valid, it injects `X-Team-Name` and `X-Api-Key` headers downstream
4. `go-router` uses the team header for budget enforcement
5. `cost-dashboard` also validates independently (defense in depth)
6. For browser access, the login page sets an HttpOnly cookie

### Pass the API Key

**As a header:**
```powershell
curl.exe -H "X-Api-Key: dev-key-123" http://localhost:3000/
```

**As a Bearer token:**
```powershell
curl.exe -H "Authorization: Bearer dev-key-123" http://localhost:3000/
```

**In a browser (query param):**
```
http://localhost:3001/dashboard?api_key=dev-key-123
```

---

## Common Operations

### Check Service Status

```powershell
docker compose -f proxyops_gateway/docker-compose.yml ps
```

### View Logs

```powershell
docker compose -f proxyops_gateway/docker-compose.yml logs -f rust-proxy
docker compose -f proxyops_gateway/docker-compose.yml logs -f cost-dashboard
```

### Restart a Single Service

```powershell
docker compose -f proxyops_gateway/docker-compose.yml restart go-router
```

### Rebuild and Restart

```powershell
docker compose -f proxyops_gateway/docker-compose.yml up -d --build cost-dashboard
```

### Full Fresh Restart (reset all data)

```powershell
docker compose -f proxyops_gateway/docker-compose.yml down -v
docker compose -f proxyops_gateway/docker-compose.yml up -d --build
```

### Stop Everything

```powershell
docker compose -f proxyops_gateway/docker-compose.yml down
```

### Health Checks

```powershell
curl.exe http://localhost:3000/health    # rust-proxy
curl.exe http://localhost:8080/health    # go-router
curl.exe http://localhost:3001/health    # cost-dashboard
curl.exe http://localhost:3010/health    # mcp-gateway
```

---

## Architecture

```
Client/AI Agent -> rust-proxy (:3000) -+-> go-router (:8080) -> upstream AI APIs
                                       |
                                       +-> mcp-gateway (:3010) -> cost-dashboard (:3001)
                                                                     |
                                                               Redis / Postgres
```

| Layer | Technology | Responsibility |
|-------|-----------|----------------|
| **rust-proxy** | Rust (Axum) | TLS termination, API key validation, request forwarding |
| **go-router** | Go | Route resolution, provider selection, circuit breaker, rate limiting, semantic caching, cost telemetry |
| **cost-dashboard** | Go | Web UI, cost aggregation, anomaly detection, prescriptive engine, alerts |
| **mcp-gateway** | Rust (Axum) | MCP protocol gateway (SSE + JSON-RPC), 11 cost-management tools |
| **Redis** | redis-stack-server | Routes, rate limits, pub/sub events, vector search cache |
| **PostgreSQL** | postgres:16-alpine | Cost history, assessments, monitoring rules, alerts |

### Response Headers

Every proxied LLM response includes:
- `X-Model-Used` — which model handled the request
- `X-Cost-Cents` — estimated cost in USD cents
- `X-Cache` — HIT or MISS (if semantic cache enabled)
- `X-Cache-Savings-Cents` — cost saved by cache hit
- `X-Cost-Routing: fallback` — when fallback provider was used

---

## API Reference

### Dashboard API (port 3001)

| Endpoint | Description |
|----------|-------------|
| `GET /api/dashboard/summary?period=24h` | Aggregate stats |
| `GET /api/dashboard/costs?period=24h` | Cost breakdown by model |
| `GET /api/dashboard/cost-timeseries?period=24h` | Hourly time-series |
| `GET /api/dashboard/anomalies?period=168h` | Anomaly detection history |
| `GET /api/dashboard/events` | SSE stream (live updates) |

### Admin API (port 3001)

| Endpoint | Methods | Description |
|----------|---------|-------------|
| `/api/admin/keys` | GET, POST, PUT, DELETE | API key CRUD |
| `/api/admin/teams` | GET, POST, DELETE | Team budget management |
| `/api/admin/budget-rules` | GET, POST, DELETE | Budget threshold rules |
| `/api/admin/pricing` | GET, POST | Model pricing |
| `/api/admin/seed-demo` | POST | Seed demo data |
| `/api/admin/escalation-policies` | GET, POST, DELETE | Alert escalation |
| `/api/budget/status?team=engineering` | GET | Team budget usage |

### MCP Tools (port 3010)

| Tool | Description |
|------|-------------|
| `get_cost_summary` | Aggregate cost for a period |
| `get_model_costs` | Per-model breakdown |
| `get_anomalies` | 3-sigma anomaly events |
| `run_assessment` | Full cost assessment |
| `run_whatif` | Single what-if scenario |
| `whatif_multi_scenario` | Compare scenarios |
| `whatif_volume_shift` | Volume change impact |
| `whatif_model_switch` | Model substitution cost |
| `get_budget_status` | Budget vs usage |
| `list_budget_rules` | Budget alert rules |
| `get_report` | Assessment report |

---

## Configuration

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `REDIS_ADDR` | `localhost:6379` | Redis connection |
| `DATABASE_URL` | `postgres://localhost:5432/cost_dashboard?sslmode=disable` | Postgres connection |
| `PORT` | `3001` | Dashboard HTTP port |
| `ANOMALY_Z_SCORE` | `3.0` | Z-score threshold |
| `MONITORING_INTERVAL` | `5m` | Spend trend check interval |
| `ANOMALY_INTERVAL` | `5m` | Anomaly detection interval |
| `RETENTION_MAX_DAYS` | `90` | Data retention period |
| `DIGEST_WEBHOOK_URL` | — | Slack/Teams webhook for cost digest |
| `DIGEST_SCHEDULE` | `24h` | Cost digest frequency |

### Data Retention

Cost entries and anomalies older than 90 days are automatically pruned daily. Configure via `RETENTION_MAX_DAYS`.
