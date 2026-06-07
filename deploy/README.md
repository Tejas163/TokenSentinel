# TokenSentinel Deployment

## Quick Start (Docker Compose)

```bash
docker compose -f proxyops_gateway/docker-compose.yml up -d
# Optional MCP gateway:
docker compose -f proxyops_gateway/docker-compose.mcp.yml up -d
```

Services:

| Service          | Port | URL                    |
|------------------|------|------------------------|
| Redis Stack      | 6379 | redis://localhost:6379 |
| PostgreSQL       | 5432 | postgres://postgres:postgres@localhost:5432/cost_dashboard |
| Go Router        | 8080 | http://localhost:8080  |
| Rust Proxy       | 3000 | http://localhost:3000  |
| Cost Dashboard   | 3001 | http://localhost:3001  |
| MCP Gateway      | 3010 | http://localhost:3010  |

### Verify

```bash
curl localhost:3000/health    # rust-proxy
curl localhost:8080/health    # go-router
curl localhost:3001/health    # cost-dashboard
```

## Environment Variables

See [README.md#environment-variables](../README.md#environment-variables) for the complete reference.

## E2E Validation

```bash
./deploy/run-e2e.sh    # Linux / macOS
./deploy/run-e2e.ps1   # Windows (PowerShell)
```

**Prerequisite**: All services must be running (`docker compose up -d`).

## Benchmark

```bash
cd benchmark
./run-benchmark.sh      # requires k6 (https://k6.io)
```

## Kubernetes

```bash
kubectl apply -f deploy/k8s/
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Cost dashboard fails to start | Check PostgreSQL is running and `DATABASE_URL` is correct |
| `401 Unauthorized` on dashboard | Set `AUTH_API_KEY` on go-router and cost-dashboard, pass `X-Api-Key` header |
| Redis connection refused | Ensure Redis is running on port 6379 (or set `REDIS_ADDR`/`REDIS_URL`) |
| Embedding cache fails | Ensure `redis-stack-server` is used (needs RediSearch module) |
| `docker compose up` fails | Ensure ports 3000, 3001 3010, 8080, 6379, 5432 are free |
