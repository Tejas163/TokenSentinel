# TokenSentinel Deployment

## Quick Start (Docker Compose)

```bash
docker compose -f proxyops_gateway/docker-compose.yml up -d
```

Services:
| Service          | Port | URL                    |
|------------------|------|------------------------|
| Redis            | 6379 | redis://localhost:6379 |
| Go Router        | 8080 | http://localhost:8080  |
| Rust Proxy       | 3000 | http://localhost:3000  |
| Cost Dashboard   | 3001 | http://localhost:3001  |
| Erlang Monitor   | —    | (background)           |

## E2E Validation

```bash
./deploy/run-e2e.sh
```

## Benchmark

```bash
cd benchmark
./run-benchmark.sh
```

## Kubernetes

```bash
kubectl apply -f deploy/k8s/
```
