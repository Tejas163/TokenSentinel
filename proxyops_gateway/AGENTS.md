# ProxyOps-Gateway — System Architecture

Multi-language AI gateway composed of four services orchestrated via Docker Compose.

## Services

| Service | Language | Port | Role |
|---------|----------|------|------|
| **rust-proxy** | Rust (axum) | `3000` | High-performance request proxy / ingress |
| **go-router** | Go (net/http) | `8080` | Request routing, load balancing, dispatch |
| **erlang-monitor** | Erlang | — | Sentinel monitoring, health checks, alerting |
| **redis** | — (redis:7-alpine) | `6379` | Shared state, rate-limit counters, pub/sub |

## Data Flow

```
Client → rust-proxy (:3000) → go-router (:8080) → upstream services
                                  ↕
                            redis (:6379)
                                  ↕
                          erlang-monitor
```

- `rust-proxy` receives all external traffic, applies TLS termination and basic auth, then forwards to `go-router`.
- `go-router` inspects the request, applies routing rules (stored in Redis), and proxies to the correct upstream.
- `erlang-monitor` watches Redis keys for health pings, runs periodic checks, and pushes alerts.
- `redis` serves as the shared state layer between all three services.

## Getting Started

```bash
docker compose up --build
```

## Project Structure

```
proxyops_gateway/
├── rust-proxy/          # Rust binary crate
│   ├── Cargo.toml
│   └── src/main.rs
├── go-router/           # Go module
│   ├── go.mod
│   └── main.go
├── erlang-monitor/      # Erlang OTP application
│   └── sentinel_monitor.erl
├── docker-compose.yml
└── AGENTS.md            # this file
```
