# TokenSentinel — Prescriptive LLM Cost Optimizer

Upload your billing CSV and get a savings plan in 30 seconds.

## Quick Start

```powershell
cd app
go run .
```

Open http://localhost:8080, upload a billing CSV, and get your report.

## Build

```powershell
go build -o tokensentinel-app .
./tokensentinel-app
```

## Deploy

```powershell
fly deploy
```
