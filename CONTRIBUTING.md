# Contributing to TokenSentinel

## Quick Start

```bash
git clone https://github.com/Tejas163/TokenSentinel.git
cd TokenSentinel
docker compose -f proxyops_gateway/docker-compose.yml up -d --build
```

## Project Structure

```
TokenSentinel/
├── proxyops_gateway/         # Core services (Docker Compose)
│   ├── rust-proxy/           # Rust edge proxy (axum, Redis)
│   ├── go-router/            # Go orchestration router
│   ├── cost-dashboard/       # Go + Postgres dashboard
│   ├── mcp-gateway/          # Rust MCP + A2A gateway
│   ├── erlang-monitor/       # Erlang OTP telemetry monitor
│   └── docker-compose.yml    # Service orchestration
├── deploy/                   # Deployment scripts & k8s manifests
├── benchmark/                # k6 load testing
├── docs/                     # Static documentation site
├── spring-boot-sdk/          # Spring Boot Starter (Java)
├── evolunet_slm/             # Python prompt optimizer
└── install.sh                # One-line installer
```

## Services

| Service | Language | Port | Tech Stack |
|---------|----------|------|------------|
| rust-proxy | Rust | 3000 | axum, redis, reqwest |
| go-router | Go | 8080 | net/http, redis |
| cost-dashboard | Go | 3001 | net/http, pgx, redis, html/template |
| mcp-gateway | Rust | 3010 | axum, SSE, reqwest |
| erlang-monitor | Erlang | — | OTP, eredis |

## Development Workflow

### 1. Pick an Issue
Check [open issues](https://github.com/Tejas163/TokenSentinel/issues).

### 2. Branch
```bash
git checkout -b feat/your-feature-name
```

### 3. Make Changes
- Follow the existing code style and patterns
- Update tests where applicable
- Run `cargo test` or `go test ./...` depending on the service

### 4. Build and Test Locally
```bash
# Build all services
docker compose -f proxyops_gateway/docker-compose.yml build

# Run full stack
docker compose -f proxyops_gateway/docker-compose.yml up -d

# Run E2E verification
bash deploy/run-e2e.sh
```

### 5. Commit
```
type(scope): brief description

- Bullet points for details if needed
- Reference issues: Fixes #123
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `security`

### 6. Push and PR
```bash
git push origin feat/your-feature-name
# Create PR on GitHub
```

## Code Guidelines

### Rust
- Use `cargo clippy` and `cargo fmt` before committing
- Use `tracing` for logging, not `println!`
- Error types should implement `std::error::Error`
- Use `axum::middleware` for cross-cutting concerns

### Go
- Use `gofmt` before committing
- Use `log.Printf` for server-side logging
- Use `net/http` standard library patterns
- Prefer `database/sql` with pgx driver for Postgres

### Erlang
- Follow OTP design principles (gen_server)
- Use `io:format` for logging (limited stdout in container)
- Include EUnit tests in the module

### HTML/CSS/JS
- Use shared styles from the cost-dashboard stylesheet
- Keep CSS in a single `<style>` block per page
- Use vanilla JS (no framework dependencies)
- Support mobile via `@media (max-width: 768px)` queries

## Testing

```bash
# All Go tests
cd proxyops_gateway/go-router && go test ./...
cd proxyops_gateway/cost-dashboard && go test ./...

# All Rust tests
cd proxyops_gateway/rust-proxy && cargo test
cd proxyops_gateway/mcp-gateway && cargo test

# Erlang tests (in Docker)
docker compose -f proxyops_gateway/docker-compose.yml run --rm erlang-monitor sh -c 'cd /app && rebar3 eunit'

# E2E
bash deploy/run-e2e.sh
```

## CI Pipeline

The CI runs on every push and PR:
1. **rust-proxy**: `cargo build` + `cargo test`
2. **go-router**: `go build` + `go test ./...`
3. **cost-dashboard**: `go build` + `go test ./...`
4. **docker**: Compose build check (service compiles)
5. **e2e**: Full docker-compose up + health probe + data flow test

## Adding a New Model

1. Add model info to `cost-dashboard/prescriptive_engine.go` `modelCatalog`
2. Add tokenizer to `go-router/token_counter.go` if needed
3. Update the docs site at `docs/`

## Security

- Never commit secrets or API keys
- Use environment variables for all configuration
- Run `docker compose exec redis redis-cli CONFIG SET requirepass <password>` in production
- Pin Docker base image versions (no `:latest`)
- Sign webhook payloads with HMAC-SHA256

## Getting Help

- Open a [GitHub issue](https://github.com/Tejas163/TokenSentinel/issues)
- Check the [docs site](https://tokensentinel.dev)
- Review the [RUNBOOK](./RUNBOOK.md) for operations questions
