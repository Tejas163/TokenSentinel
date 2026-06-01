# TokenSentinel Demo

A self-contained interactive demo showcasing TokenSentinel's core features.

## Quick Start

```powershell
# From project root:
.\demo\demo.ps1
```

The demo will:
1. Verify all services are running
2. Inject realistic multi-model cost data (gpt-4, claude-3-opus, gpt-3.5-turbo, gemini-pro)
3. Query the cost dashboard API
4. Open the HTML dashboard in your browser
5. Show cost breakdowns by model
6. Test the route resolution system
7. Print a summary report

## Prerequisites

- Docker & Docker Compose
- PowerShell 5.1+

## Output

See `demo-results.txt` for sample output from a successful run.

```
=== HEALTH CHECK ===
  ✅ Redis           → PONG
  ✅ Go Router       → {"status":"ok"}  
  ✅ Rust Proxy      → {"status":"ok"}
  ✅ Erlang Monitor  → Heartbeat: 1780299326
  ✅ Cost Dashboard  → HTTP 200

=== COST SUMMARY (24h) ===
  Requests:     8
  Total Tokens: 3,400
  Unique Models: 4

=== BY MODEL ===
  claude-3-opus     → 4 req, 2,180 tokens
  gpt-4             → 3 req, 920 tokens
  gpt-3.5-turbo     → 1 req, 135 tokens
  gemini-pro        → 1 req, 180 tokens

=== ROUTE TEST ===
  ✅ Route stored: /v1/chat/completions
```
