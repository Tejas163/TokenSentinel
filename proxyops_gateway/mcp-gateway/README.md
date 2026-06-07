# TokenSentinel MCP Server

MCP server for LLM token cost observability, model routing, budget enforcement, and prescriptive cost optimization. Provides 11 tools that give AI agents visibility into their own cost footprint.

## Tools (11)

| Tool | Description |
|------|-------------|
| `get_cost_summary` | Aggregate token cost for a period (1h/6h/24h/72h/168h) |
| `get_model_costs` | Per-model cost breakdown |
| `get_anomalies` | 3-sigma anomaly detection on usage spikes |
| `run_assessment` | Full cost assessment with model substitution, infra, provider recommendations |
| `run_whatif` | Single what-if scenario (volume, model mix) |
| `whatif_multi_scenario` | Multiple scenarios compared side-by-side |
| `whatif_volume_shift` | Volume increase/decrease impact on costs |
| `whatif_model_switch` | Model substitution cost impact |
| `get_budget_status` | Team budget vs actual usage |
| `list_budget_rules` | Budget threshold alert rules |
| `get_report` | Complete assessment report |

## Quick Start

```bash
# 1. Start the server with its dependencies
docker compose -f docker-compose.mcp.yml up -d

# 2. Check health
curl http://localhost:3010/health

# 3. List tools (MCP HTTP transport)
curl -X POST http://localhost:3010/mcp \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-api-key" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# 4. Call a tool
curl -X POST http://localhost:3010/mcp \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-api-key" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_cost_summary","arguments":{"period":"24h"}}}'
```

## Transport

Uses MCP HTTP transport:
- **SSE** at `GET /mcp` — server-sent events for streaming tool results
- **POST** at `POST /mcp` — JSON-RPC `tools/call` requests
- **Health** at `GET /health`

## Architecture

```
MCP Client (Claude Desktop, Cursor, VS Code, etc.)
  │
  ├── SSE → GET /mcp (streaming session)
  │
  └── JSON-RPC → POST /mcp
        │
        ├── tools/list    → static tool definitions
        ├── tools/call    → dispatch to cost-dashboard REST API
        └── ...
              │
              ▼
        cost-dashboard (:3001)
              │
              ├── Redis — shared state, pub/sub
              └── PostgreSQL — cost entries, assessments, rules
```

The MCP server is stateless — it proxies tool calls to the cost-dashboard REST API. This keeps the server lean while the dashboard handles data persistence and analysis.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `MCP_API_KEY` | Yes | — | Comma-separated API keys for MCP auth |
| `DASHBOARD_URL` | Yes | `http://localhost:3001` | Cost dashboard base URL |
| `DASHBOARD_API_KEY` | Yes | — | API key for dashboard upstream calls |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection (for readiness) |

## Client Examples

### Claude Desktop

```json
{
  "mcpServers": {
    "tokensentinel": {
      "url": "http://localhost:3010/mcp",
      "headers": {
        "X-Api-Key": "your-api-key"
      }
    }
  }
}
```

### Cursor

```
Settings → MCP Servers → Add:
  URL: http://localhost:3010/mcp
  Headers: { "X-Api-Key": "your-api-key" }
```

### VS Code (GitHub Copilot)

```json
{
  "github.copilot.mcp-servers": {
    "tokensentinel": {
      "url": "http://localhost:3010/mcp",
      "headers": {
        "X-Api-Key": "your-api-key"
      }
    }
  }
}
```

### OpenAI Responses API

```python
import requests

response = requests.post(
    "https://api.openai.com/v1/responses",
    headers={"Authorization": "Bearer sk-..."},
    json={
        "model": "gpt-4o",
        "input": "what's my cost trend for the last 24 hours?",
        "tools": [{
            "type": "function",
            "function": {
                "name": "tokensentinel_mcp",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "server_url": {"type": "string"},
                        "headers": {"type": "object"},
                        "body": {"type": "object"}
                    }
                }
            }
        }]
    }
)
```

## Standalone Deployment

```yaml
# docker-compose.mcp.yml
services:
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: cost_dashboard
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports: ["5432:5432"]

  cost-dashboard:
    build: ./cost-dashboard
    ports: ["3001:3001"]
    environment:
      REDIS_ADDR: redis:6379
      DATABASE_URL: postgres://postgres:postgres@postgres:5432/cost_dashboard?sslmode=disable
      AUTH_API_KEY: your-api-key
    depends_on: [redis, postgres]

  mcp-server:
    build: .
    ports: ["3010:3010"]
    environment:
      MCP_API_KEY: your-api-key
      DASHBOARD_URL: http://cost-dashboard:3001
      DASHBOARD_API_KEY: your-api-key
      REDIS_URL: redis://redis:6379
    depends_on: [cost-dashboard]
```

## Security

- **API key auth**: `X-Api-Key` header or `Authorization: Bearer` on all MCP endpoints
- **Team data scoping**: `AGENT_TEAM_MAP` env var maps keys to teams, scoping cost data per team
- **Budget enforcement**: Tool calls are blocked for teams that exceed their budget
- **No PII**: Tools return aggregated cost data only — no prompt text or response content crosses the boundary

## Building

```bash
cargo build --release
```

Requires `pkg-config` and `libssl-dev` on Linux, or use the multi-stage Docker build.
