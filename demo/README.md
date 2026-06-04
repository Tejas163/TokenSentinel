# TokenSentinel Demo

Self-contained demos showcasing TokenSentinel's core features.

## Quick Start (Basic)

```powershell
.\demo\demo.ps1
```

Verifies services, injects sample traffic, queries dashboard, tests routes.

## End-to-End Demo (Full)

```powershell
.\demo\end-to-end.ps1
```

Demonstrates the complete pipeline across 12 phases:

| Phase | What it shows |
|-------|---------------|
| 1 | Environment check (Redis, 3 services) |
| 2 | Inject 15 realistic cost records across 7 models and 4 teams |
| 3 | Create a prescriptive assessment (cloud infra + team + spend) |
| 4 | Run the prescriptive engine → cost breakdown + recommendations + ROI |
| 5 | Fetch report JSON endpoint |
| 6 | CSV + PDF report exports |
| 7 | What-If simulator (50% volume increase) |
| 8 | Starter templates (Startup, Mid-size, Enterprise) |
| 9 | Monitoring rule creation + spend spike injection + alert detection |
| 10 | Savings tracking check |
| 11 | Cost trend data per model |
| 12 | Version management (update → new version → version history) |

## Prerequisites

- Docker & Docker Compose (services must be running)
- PowerShell 5.1+

## Core Demo

```
=== HEALTH CHECK ===
  ✅ Redis           → PONG
  ✅ Go Router       → {"status":"ok"}  
  ✅ Rust Proxy      → {"status":"ok"}
  ✅ Erlang Monitor  → Heartbeat: 1780299326
  ✅ Cost Dashboard  → HTTP 200
```
