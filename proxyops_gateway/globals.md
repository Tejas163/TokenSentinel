# ProxyOps-Gateway — Redis Key Naming Convention

## Pattern Reference

| Key Pattern                     | Purpose              | Written By         | Read By            | TTL   |
|---------------------------------|----------------------|--------------------|--------------------|-------|
| `sentinel:{request_id}:cost`    | Telemetry cost entry | erlang-monitor     | erlang-monitor     | 24h   |
| `proxy:{session_id}:blocked`    | Blocked session flag | rust-proxy         | rust-proxy         | —     |
| `ratelimit:{key}`               | Rate-limit counter   | rust-proxy         | rust-proxy         | 60s   |
| `routes:{pattern}`              | Routing rule         | go-router          | go-router          | —     |
| `health:{service}`              | Heartbeat timestamp  | erlang-monitor     | cost-dashboard     | 30s   |

## Key Components

| Placeholder      | Description                          | Example Value                  |
|------------------|--------------------------------------|--------------------------------|
| `{request_id}`   | UUID v4 generated at the edge        | `a1b2c3d4-e5f6-7890-abcd-ef0123456789` |
| `{session_id}`   | Client session identifier            | `sess_abc123`                  |
| `{key}`          | Client IP or API key hash            | `md5(203.0.113.42)`            |
| `{pattern}`      | URL path glob pattern                | `/v1/chat/completions`         |
| `{service}`      | Service name                         | `rust-proxy`, `go-router`, `erlang-monitor`, `cost-dashboard` |

---

## Cost Dashboard — SQLite Schema

Database: `cost_dashboard.db` (persistent volume mounted at `/data`)

```sql
CREATE TABLE cost_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL UNIQUE,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    timestamp TEXT NOT NULL,         -- RFC 3339 from upstream
    ingested_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_cost_model     ON cost_entries(model);
CREATE INDEX idx_cost_timestamp ON cost_entries(timestamp);
```

### API Endpoints

| Method | Path                          | Description                                |
|--------|-------------------------------|--------------------------------------------|
| GET    | `/`                           | HTML dashboard page                        |
| GET    | `/api/dashboard/costs`        | Cost by model group (query: `?period=1h`)  |
| GET    | `/api/dashboard/summary`      | Aggregate stats (query: `?period=24h`)     |
