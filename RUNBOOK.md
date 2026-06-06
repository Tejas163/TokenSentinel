# RUNBOOK — TokenSentinel Operations

## Service Overview

| Service | Port | Language | Health Check |
|---------|------|----------|-------------|
| Rust Proxy (ingress) | 3000 | Rust/axum | `GET /health` → `{"status":"ok"}` |
| Go Router | 8080 | Go | `GET /health` → `{"status":"ok"}` |
| Cost Dashboard | 3001 | Go | `GET /health` → `{"status":"ok"}` |
| MCP Gateway | 3010 | Rust/axum | `GET /health` → `{"status":"ok"}` |
| Cost Dashboard (monitoring) | 3001 | Go | Spend trends, savings detection, alert dispatch |
| Redis | 6379 | — | `PING` → `PONG` |
| Postgres | 5432 | — | `pg_isready` |

## Common Tasks

### Check all services
```bash
docker compose -f proxyops_gateway/docker-compose.yml ps
```

### Tail logs for a service
```bash
docker compose -f proxyops_gateway/docker-compose.yml logs -f rust-proxy
docker compose -f proxyops_gateway/docker-compose.yml logs -f cost-dashboard
```

### Restart a single service
```bash
docker compose -f proxyops_gateway/docker-compose.yml restart cost-dashboard
```

### Rebuild and restart
```bash
docker compose -f proxyops_gateway/docker-compose.yml up -d --build <service>
```

### Full restart (all services, fresh data)
```bash
docker compose -f proxyops_gateway/docker-compose.yml down -v
docker compose -f proxyops_gateway/docker-compose.yml up -d --build
```

### Check Redis state
```bash
docker compose exec redis redis-cli KEYS 'health:*'
docker compose exec redis redis-cli KEYS 'sentinel:*'
docker compose exec redis redis-cli KEYS 'budget:*'
```

### Check cost data in Postgres
```bash
docker compose exec postgres psql -U postgres -d cost_dashboard -c "SELECT COUNT(*) FROM cost_entries;"
docker compose exec postgres psql -U postgres -d cost_dashboard -c "SELECT model, SUM(input_tokens+output_tokens) FROM cost_entries GROUP BY model;"
```

## Health Verification

Run the E2E health check:
```bash
bash deploy/run-e2e.sh
```

Or manually:
```bash
# Redis
docker compose exec redis redis-cli PING


# Rust proxy
curl -s http://localhost:3000/health

# Go router
curl -s http://localhost:8080/health

# Cost dashboard
curl -s -o /dev/null -w "%{http_code}" http://localhost:3001/health

# MCP gateway
curl -s http://localhost:3010/health
```

## Seed Demo Data
```bash
curl -X POST http://localhost:3001/api/admin/seed-demo \
  -H "X-Api-Key: dev-key-123"
```

## Add a Route
```bash
docker compose exec redis redis-cli SET routes:/chat \
  '{"pattern":"/chat","providers":[{"url":"https://api.openai.com/v1/chat/completions","model":"gpt-4","weight":3,"timeout":30}]}'
```

## Configuration

### Environment Variables (.env or docker-compose.yml)

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | `redis://127.0.0.1:6379` | Redis connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis address (Go services) |
| `REDIS_PASSWORD` | — | Redis ACL password |
| `DATABASE_URL` | `postgres://localhost:5432/cost_dashboard?sslmode=disable` | Postgres DSN |
| `AUTH_API_KEY` | — | API key for dashboard auth |
| `MCP_API_KEY` | — | Comma-separated API keys for MCP auth |
| `DIGEST_WEBHOOK_URL` | — | Slack/Teams webhook for cost digest |
| `DIGEST_SCHEDULE` | `24h` | Digest interval |
| `MONITORING_INTERVAL` | `5m` | Spend trend check interval |
| `ANOMALY_INTERVAL` | `5m` | Anomaly detection interval |
| `ANOMALY_Z_SCORE` | `3.0` | Z-score threshold for anomalies |
| `RETENTION_MAX_DAYS` | `90` | Days to retain cost data |

## Scaling

- **Rust Proxy**: Stateless. Scale horizontally behind a load balancer.
- **Go Router**: Stateless. Scale horizontally (routes in Redis).
- **Cost Dashboard**: Stateful (Postgres). Read replicas for dashboard queries; single writer.
- **Erlang Monitor**: Stateless. Single instance sufficient (lightweight).
- **MCP Gateway**: Stateless. Scale with proxy.
- **Redis**: Single instance for dev. Redis Cluster or Sentinel for HA in production.
- **Postgres**: Single instance for dev. Streaming replicas for HA in production.

## Troubleshooting

### "Redis connection refused"
Check Redis is running: `docker compose ps redis`. If the container is up, verify `REDIS_URL`/`REDIS_ADDR` is correct.

### "Dashboard shows no data"
1. Seed demo data: `curl -X POST http://localhost:3001/api/admin/seed-demo -H "X-Api-Key: dev-key-123"`
2. Check Redis cost entries: `docker compose exec redis redis-cli KEYS 'sentinel:*'`

### "401 Unauthorized"
Ensure `X-Api-Key` header matches `AUTH_API_KEY` or `MCP_API_KEY` env var.

### "Circuit breaker open"
The Rust proxy circuit breaker opens after 5 failures. It auto-recovers after 30s. Check the upstream service is healthy.

### "MCP agent can't connect"
1. Verify MCP API key: `MCP_API_KEY` env var on both rust-proxy and mcp-gateway
2. Check MCP gateway health: `curl -s http://localhost:3010/health`
3. Verify rust-proxy forwards `/mcp/*` correctly

### Postgres connection issues
```
docker compose logs postgres
docker compose exec postgres pg_isready -U postgres -d cost_dashboard
```

## Backup

### Postgres
```bash
docker compose exec -T postgres pg_dump -U postgres cost_dashboard > backup_$(date +%Y%m%d).sql
```

### Redis
```bash
docker compose exec redis redis-cli SAVE
# dump.rdb is stored in the container's data volume
```

## Incident Response

1. **Service down**: Check logs: `docker compose logs --tail=100 <service>`
2. **High latency**: Check circuit breaker state. Check upstream provider latency.
3. **Budget breach**: Review budget rules: `GET /api/admin/budget-rules`
4. **Data loss**: Restore from Postgres backup. Redis cost entries are ephemeral (24h TTL) — the dashboard re-ingests from Postgres.
