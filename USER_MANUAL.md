# TokenSentinel User Manual

## What It Is

TokenSentinel is an AI gateway that sits between your LLM clients and upstream AI providers. It routes requests, tracks every token + cost, detects anomalies, enforces budgets, and gives you a real-time dashboard — all without modifying your app code.

You configure routes in Redis. Clients point at `rust-proxy:3000`. Everything else is automatic.

---

## Architecture

```
┌──────────────┐     ┌──────────────────────────────────────────────────────┐
│  LLM Clients │────▶│  rust-proxy :3000                                    │
│  (curl, SDK) │     │  TLS termination, rate limit, circuit breaker        │
└──────────────┘     └────────────┬─────────────────────────────────────────┘
                                  │
                                  ▼
                         ┌──────────────────────────────────────────────────┐
                         │  go-router :8080                                 │
                         │  Route resolution (Redis), provider selection,   │
                         │  retry/backoff, circuit breaker, token counting, │
                         │  model selection, budget check, cost recording   │
                         │  Adds X-Model-Used, X-Cost-Cents to responses    │
                         └──────┬──────────────────┬────────────────────────┘
                                │                  │
                                ▼                  ▼
                      ┌──────────────┐    ┌──────────────┐
                      │  Upstream AI  │    │    Redis     │
                      │  Providers    │    │  :6379       │
                      │  (OpenAI,etc) │    │  Routes,     │
                      └──────────────┘    │  cost data,  │
                                          │  budgets,    │
                                          │  pub/sub     │
                                          └──────┬───────┘
                                                 │
                                    ┌────────────┼────────────┐
                                    ▼            ▼            ▼
                           ┌────────────┐ ┌───────────┐ ┌────────────┐
                            │ PostgreSQL │ │ cost-      │
                            │ (cost      │ │ dashboard  │
                            │  history)  │ │ :3001      │
                            └────────────┘ │ Web UI +   │
                                           │ Admin API  │
                                           └────────────┘
```

### Services at a Glance

| Service | Port | Language | Job |
|---------|------|----------|-----|
| `rust-proxy` | 3000 | Rust | TLS, rate limiting, upstream circuit breaker |
| `go-router` | 8080 | Go | Route resolution, load balancing, retry, cost tracking |
| `cost-dashboard` | 3001 | Go | Web UI, admin API, PostgreSQL persistence |
| `cost-dashboard` | 3001 | Go | Web UI, admin API, PostgreSQL, monitoring engine (spend trends, savings detection, alerts) |
| `redis` | 6379 | — | Shared state: routes, costs, budgets, pub/sub |
| `postgres` | 5432 | — | Historical cost data, teams, budget rules |

### Data Flow

1. Client sends request to `rust-proxy:3000`
2. `rust-proxy` forwards to `go-router:8080` with original headers
3. `go-router` looks up the route in Redis (`routes:/path`), picks a provider (weighted or auto-model), checks budget, proxies to upstream, records cost to Redis
4. `go-router` publishes `cost:{request_id}` to Redis `health:events`
5. `cost-dashboard` subscribes to `health:events`, reads cost data from Redis, inserts into PostgreSQL, updates Redis budget counters
6. Browser SSE connection to `cost-dashboard:3001/api/dashboard/events` streams live cost + anomaly events

---

## Getting Started

### Prerequisites

- Docker + Docker Compose
- Redis 7+
- PostgreSQL 15+ (or Docker image)

### Start Everything

```bash
cd proxyops_gateway
docker compose up --build -d
```

This starts all 6 services. The default docker-compose uses Postgres (not SQLite).

### Check It's Running

```bash
# Health endpoints
curl localhost:3000/health          # rust-proxy
curl localhost:8080/health          # go-router
curl localhost:3001/health          # cost-dashboard
curl localhost:8080/metrics         # Go router expvar metrics

# Redis is alive
docker compose exec redis redis-cli PING
```

### Add a Route

Routes are JSON objects stored in Redis as `routes:{path}`.

```bash
# Route /chat to two OpenAI providers with weighted load balancing
redis-cli SET routes:/chat '{
  "pattern": "/chat",
  "providers": [
    {"url": "https://api.openai.com/v1/chat/completions", "model": "gpt-4", "weight": 3, "timeout": 30},
    {"url": "https://api.openai.com/v1/chat/completions", "model": "gpt-3.5-turbo", "weight": 1, "timeout": 15}
  ]
}'
```

### Make a Request

```bash
curl -X POST http://localhost:3000/chat \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-api-key" \
  -H "X-Team-Name: engineering" \
  -d '{"messages":[{"role":"user","content":"Hello"}]}'
```

Response includes:
- `X-Model-Used`: which model handled it
- `X-Cost-Cents`: estimated cost in cents

---

## Environment Variables

### go-router (:8080)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | — | Redis AUTH password |
| `AUTH_API_KEY` | — | If set, all routes except `/health` and `/metrics` require this key via `X-Api-Key` or `Authorization: Bearer` |
| `BUDGET_TEAM_NAME` | — | Default team name for all cost records (if no `X-Team-Name` header) |

### cost-dashboard (:3001)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | — | Redis AUTH password |
| `DATABASE_URL` | `postgres://localhost:5432/cost_dashboard?sslmode=disable` | PostgreSQL connection string |
| `AUTH_API_KEY` | — | API key for admin/dashboard endpoints (same as go-router) |
| `DIGEST_WEBHOOK_URL` | — | Slack/Teams incoming webhook URL for cost digest |
| `DIGEST_SCHEDULE` | `24h` | Digest interval (e.g., `24h`, `168h`) |

### rust-proxy (:3000)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | `redis://127.0.0.1:6379` | Redis connection URL |
| `GO_ROUTER_URL` | `http://127.0.0.1:8080` | Upstream Go router address |
| `REDIS_PASSWORD` | — | Redis AUTH password |



---

## Daily Usage

### Cost Dashboard (Web UI)

Open `http://localhost:3001` in a browser.

Features:
- **Summary cards**: total requests, tokens, unique models, avg tokens/req
- **Cost by model table**: breakdown per model with input/output token counts
- **Period selector**: 1H / 6H / 24H / 3D / 7D
- **Live badge**: real-time SSE connection — new cost rows appear instantly
- **Anomaly alerts**: when a request exceeds 3σ for its model, a red banner appears at the top

### Admin API (cost-dashboard)

All admin endpoints require `X-Api-Key: <key>` if `AUTH_API_KEY` is set.

#### Teams

```bash
# List teams
curl localhost:3001/api/admin/teams

# Add a team (monthly budget in tokens)
curl -X POST localhost:3001/api/admin/teams \
  -H "Content-Type: application/json" \
  -d '{"name":"engineering","monthly_token_budget":10000000,"period":"30d"}'

# Remove a team
curl -X DELETE 'localhost:3001/api/admin/teams?id=1'
```

#### Budget Alert Webhooks

```bash
# List rules
curl localhost:3001/api/admin/budget-rules

# Add a rule
curl -X POST localhost:3001/api/admin/budget-rules \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","max_tokens":5000000,"period":"24h","webhook_url":"https://hooks.slack.com/..."}'

# Delete a rule
curl -X DELETE 'localhost:3001/api/admin/budget-rules?id=1'
```

#### Check Budget Status

```bash
curl 'localhost:3001/api/budget/status?team=engineering'
# Returns: {"team":"engineering","budgeted":true,"limit":10000000,"used":3420000,"remaining":6580000,"over_budget":false}
```

### Dashboard API

```bash
# Cost summary
curl 'localhost:3001/api/dashboard/summary?period=24h'

# Cost breakdown by model
curl 'localhost:3001/api/dashboard/costs?period=24h'

# Anomalies
curl 'localhost:3001/api/dashboard/anomalies?period=24h'
# Lists requests where total_tokens > µ + 3σ for their model
```

### Route Configuration

Routes are JSON in Redis. Full schema:

```
routes:{path} → {
  "pattern": "/chat",
  "providers": [
    {
      "url": "https://...",
      "model": "gpt-4",
      "weight": 3,
      "timeout": 30
    }
  ],
  "auto_model": false   // set true for autonomous model selection
}
```

- `auto_model: true` — router analyzes prompt length and picks the capability tier (cheap/medium/capable) automatically
- `weight` — higher weight = more traffic. Default: 1

### Autonomous Model Selection

When `auto_model: true` on a route, the Go router picks the provider whose model best matches the prompt complexity:

| Prompt Length | Tier Used |
|--------------|-----------|
| < 500 chars | Cheap (gpt-3.5, haiku, llama-8b, etc.) |
| 500–2000 chars | Medium (gpt-4o-mini, sonnet, etc.) |
| > 2000 chars | Capable (gpt-4, opus, llama-70b, etc.) |

If multiple providers match the needed tier, the first one is used. If none match, the closest tier is chosen.

### Budget-Aware Routing

When a request has `X-Team-Name: <team>` header and the team's monthly budget is exceeded, the Go router automatically reroutes to the cheapest available provider instead of returning an error.

To configure:
```bash
# Add team with budget
curl -X POST localhost:3001/api/admin/teams \
  -H "Content-Type: application/json" \
  -d '{"name":"engineering","monthly_token_budget":5000000}'

# The router checks Redis for budget:team:<name>:used vs :limit counters
```

### Cost Annotations

Every proxied response includes:
- `X-Model-Used`: the model that handled the request
- `X-Cost-Cents`: estimated cost in cents (based on per-model input/output pricing)

### Cost Digest (Slack/Teams)

Set `DIGEST_WEBHOOK_URL` and `DIGEST_SCHEDULE` on the cost dashboard. It posts a periodic cost summary in Slack-compatible attachment format:

```
TokenSentinel Cost Digest (24h0m0s)

Cost Summary
  gpt-4 (142 req)    2850K tokens (in: 1200K, out: 1650K)
  gpt-3.5-turbo (89 req)  420K tokens (in: 300K, out: 120K)
  ----------------------------------------
  Total: 3270K tokens
```

### Caching

Route configs are cached in-memory (Go router) for 60 seconds with Redis fallback. No manual cache invalidation needed.

---

## Anomaly Detection

Every 5 minutes, the cost dashboard runs a 3σ check on the last 24 hours of data (per-model):

- Computes mean (`µ`) and sample standard deviation (`σ`) of `(input_tokens + output_tokens)` per request
- Flags any request where `total_tokens > µ + 3σ`
- Logs the anomaly, publishes to Redis `anomaly:events` channel, and broadcasts to all SSE-connected dashboards

Config: the check window is hardcoded to 24h. The background interval is 5 minutes.

---

## Data Retention

Cost entries older than 90 days are automatically pruned daily. The cost dashboard runs a `DELETE FROM cost_entries WHERE timestamp < NOW() - 90d` every 24 hours.

---

## Security

- Set `AUTH_API_KEY` on both go-router and cost-dashboard to require `X-Api-Key` or `Authorization: Bearer <key>` on all non-health endpoints
- Set `REDIS_PASSWORD` on all services to enable Redis AUTH
- The docker-compose internal network isolates services by default
- `rust-proxy` can terminate TLS (configure certificates in its Dockerfile)

---

## Prescriptive Engine

TokenSentinel includes a prescriptive engine that analyzes your cloud infrastructure, model usage, and team composition to generate cost-saving recommendations.

### Creating an Assessment

1. **POST** `/api/prescriptive/assessments` with your company details, GPU config, monthly volumes, providers used, and team composition
2. Optionally fetch one of three starter templates via **GET** `/api/prescriptive/templates` to pre-fill common setups
3. Set `"source": "live"` to auto-pull existing cost data from the gateway, or `"source": "manual"` to provide your own spend figures

### Running the Engine

**POST** `/api/prescriptive/assessments/{id}/run` triggers the analysis:

- **Cost breakdown** — Per-model and per-provider spend with token volumes
- **Model substitution** — Identifies opportunities to replace expensive models with cheaper equivalents (e.g., GPT-4 → Llama-3-70B for simpler tasks)
- **Infra downsizing** — Detects underutilized GPU capacity and recommends cluster size reductions
- **Provider switching** — Estimates savings from moving workloads to self-hosted or alternative providers
- **Batch optimization** — Identifies workloads suitable for off-peak/batch processing

### Viewing the Report

Navigate to `/api/prescriptive/report/{id}` in your browser to see:

- **Executive Summary** — Current spend, projected spend, savings rate, top recommendation
- **Cost Breakdown** — Table by model with team drill-down toggle
- **Recommendations** — Sorted by priority (high/medium/low) with monthly savings and payback period
- **What-If Simulator** — Interactive sliders to adjust request volume and input/output token ratio; recalculates projections client-side in real-time
- **Version History** — Compare versions side-by-side using the overlay picker

### Exporting Reports

- **CSV** — `/api/prescriptive/report/{id}/csv` downloads cost projections and recommendations as a spreadsheet
- **PDF** — `/api/prescriptive/report/{id}/pdf` generates a formatted document with executive summary, cost table, and top recommendations

### CSV Import

Import billing data from any provider via **POST** `/api/prescriptive/import/csv`. Send a multipart form with:

| Field | Description |
|-------|-------------|
| `file` | The CSV file |
| `assessment_id` | Target assessment ID (optional) |
| `has_header` | `"true"` or `"false"` |
| `model_column` | Column index for model name (0-based) |
| `input_tokens_column` | Column index for input tokens |
| `output_tokens_column` | Column index for output tokens |
| `cost_column` | Column index for cost (optional) |

---

## Continuous Optimization

The continuous optimization engine runs as background goroutines inside the cost-dashboard service, monitoring spend patterns, detecting savings, and dispatching alerts — no manual action needed.

### How It Works

Three goroutines run continuously:

| Goroutine | Interval | Purpose |
|-----------|----------|---------|
| `monitorSpendTrends` | 5 min | Compares each model's spend against rolling 7-day baseline. Fires alert if spend exceeds configured threshold (default: +20% and >$100). |
| `trackSavings` | 5 min | Checks for cost drops >30% vs prior period. Records a savings event and fires an info alert. |
| `sendAlerts` | 30 sec | Dispatches pending alerts to all configured channels (webhook, email, dashboard). |

### Configuring Alert Rules

By default, alerts fire for any model with a 20%+ spend increase that exceeds $100 absolute. Create per-model rules via **POST** `/api/monitoring/rules`:

```json
{
  "model": "gpt-4o",
  "pct_threshold": 15,
  "abs_threshold": 200,
  "period": "7d",
  "webhook_url": "https://hooks.slack.com/services/...",
  "email_to": "admin@company.com"
}
```

Use `"model": "*"` for a catch-all rule. List, update, and delete rules via `/api/monitoring/rules`.

### Alert Channels

- **Dashboard UI** — Toast notifications appear in the top-right corner (auto-dismiss 30s). Active alerts show in a banner panel with acknowledge buttons.
- **Webhook** — Signed POST to configured URL (HMAC-SHA256, same pattern as budget webhooks).
- **Email** — Configure `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `FROM_ADDR` env vars. Supports port 465 (TLS) and 587/25 (STARTTLS). Optional SendGrid upgrade via `SENDGRID_API_KEY`.

### Savings Detection

Every 5 minutes, the engine compares each model's current 7-day cost against the prior 7-day cost. If a drop of 30%+ is detected (and no savings event was recorded in the last 7 days), it:

1. Creates a `savings_event` with the detected amount and confidence level
2. Fires an info alert: "Savings detected on [model]: $X/mo (Y% drop)"
3. Records the event in the savings tracking table

View savings events via **GET** `/api/monitoring/savings`. View per-model cost trends via **GET** `/api/monitoring/trends/{model}?period=30d`.

### Alert Lifecycle

1. Engine detects condition → alert created (30-min cooldown prevents duplicates)
2. `sendAlerts` dispatches to all channels within 30 seconds
3. User can **acknowledge** (removes from active view) or **dismiss** (permanently hides)
4. Alerts persist in the database for audit/history

---

## Quick Troubleshooting

| Symptom | Check |
|---------|-------|
| `401 Unauthorized` | `AUTH_API_KEY` is set — add `X-Api-Key` header |
| `503 Service Unavailable` | Circuit breaker open (5 failures in 30s). Wait or check upstream health. |
| No cost data in dashboard | Check Redis has `sentinel:{request_id}:cost` keys. Check `health:events` pub/sub. |
| Anomaly alerts not firing | Need at least a few requests per model to compute stddev. Single-entry models won't trigger. |
| Webhook not firing | `checkBudgets` runs every 5 min. Cooldown is 30 min. |
| Digest not sending | Set `DIGEST_WEBHOOK_URL` and `DIGEST_SCHEDULE` env vars. |
