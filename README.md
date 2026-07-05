# TokenSentinel

**Upload your LLM billing CSV. Get a savings plan in 30 seconds.**

[![CI](https://github.com/Tejas163/TokenSentinel/actions/workflows/ci.yml/badge.svg)](https://github.com/Tejas163/TokenSentinel/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Tejas163/TokenSentinel)](https://goreportcard.com/report/github.com/Tejas163/TokenSentinel)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

TokenSentinel is a single Go binary that analyzes your LLM provider billing data and gives you a professional PDF report with:

- Cost breakdown by model and provider
- Model substitution recommendations (save by switching to equivalent models)
- Provider switching analysis (compare cloud vs self-hosted)
- Anomaly detection and waste identification
- Batch optimization opportunities
- Infrastructure downsizing recommendations

No Docker. No Redis. No Postgres. No signup. Just a download, a CSV, and a report.

## Quick Start

```bash
# Download the binary for your platform from GitHub Releases
# Or run from source:
cd app
go run .
```

Open http://localhost:8080, upload your billing CSV, and get your report.

### CSV Format

Required columns (auto-detected):
- `model` (or `model_name`, `model_id`, `llm`)
- `input_tokens` (or `prompt_tokens`, `input`)
- `output_tokens` (or `completion_tokens`, `output`)
- `cost` (or `total_cost`, `amount`)
- `provider` (optional — or `service`, `vendor`)

[Download sample CSV](http://localhost:8080/sample-billing.csv)

## Features

- **Auto-detect column headers** — works with OpenAI, Anthropic, and standard billing CSVs
- **Multi-currency support** — 20+ currencies with automatic FX rates
- **Model equivalence engine** — finds cheaper equivalent models across providers
- **GPU infrastructure analysis** — detects underutilized GPU clusters
- **Batch optimization** — flags workloads that could save via off-peak/batch processing
- **PDF report** — professional downloadable report with executive summary
- **API key import** — optionally connect via API key for live billing data (OpenAI, Anthropic)

## Build

```bash
cd app
go build -o tokensentinel .
```

## Single Binary

That's the whole product. A single binary that parses your billing data and gives you actionable savings recommendations. No accounts, no setup, no infrastructure.

## Deploy (optional)

```bash
cd app
fly deploy
```
