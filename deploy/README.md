# TokenSentinel Deployment

## Quick Start (Docker Compose)

```bash
docker compose -f proxyops_gateway/docker-compose.yml up -d
```

Services:

| Service          | Port | URL                    |
|------------------|------|------------------------|
| Redis            | 6379 | redis://localhost:6379 |
| PostgreSQL       | 5432 | postgres://postgres:postgres@localhost:5432/cost_dashboard |
| Go Router        | 8080 | http://localhost:8080  |
| Rust Proxy       | 3000 | http://localhost:3000  |
| Cost Dashboard   | 3001 | http://localhost:3001  |
| Erlang Monitor   | —    | (background)           |

### Verify

```bash
curl localhost:3000/health    # rust-proxy
curl localhost:8080/health    # go-router
curl localhost:3001/health    # cost-dashboard
```

## Environment Variables

| Variable | Services | Default | Required |
|----------|----------|---------|----------|
| `DATABASE_URL` | cost-dashboard | `postgres://localhost:5432/cost_dashboard?sslmode=disable` | Yes for production |
| `AUTH_API_KEY` | go-router, cost-dashboard | — (auth disabled) | Recommended |
| `REDIS_ADDR` | go-router, cost-dashboard | `localhost:6379` | No |
| `REDIS_URL` | rust-proxy, erlang-monitor | `redis://127.0.0.1:6379` | No |
| `REDIS_PASSWORD` | go-router, cost-dashboard, erlang-monitor | — | If Redis AUTH enabled |
| `GO_ROUTER_URL` | rust-proxy | `http://127.0.0.1:8080` | No |
| `BUDGET_TEAM_NAME` | go-router | — | No |
| `DIGEST_WEBHOOK_URL` | cost-dashboard | — | For Slack/Teams digest |
| `DIGEST_SCHEDULE` | cost-dashboard | `24h` | No |

See `proxyops_gateway/docker-compose.yml` for the full dev setup with defaults.

## E2E Validation

```bash
# Linux / macOS
./deploy/run-e2e.sh

# Windows (PowerShell)
./deploy/run-e2e.ps1
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
| `401 Unauthorized` on dashboard | Set `AUTH_API_KEY` on both go-router and cost-dashboard, pass `X-Api-Key` header |
| Redis connection refused | Ensure Redis is running on `localhost:6379` (or set `REDIS_ADDR`/`REDIS_URL`) |
| `docker compose up` fails | Ensure ports 3000, 8080, 3001, 6379, 5432 are free |
