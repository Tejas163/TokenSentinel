# TokenSentinel — One-Pager

**Upload your LLM billing CSV. Get a savings plan in 30 seconds.**

---

## The Problem

AI spend is the fastest-growing line item in every engineering budget, and nobody has control.

Companies burning $10k-$500k/month on LLM APIs have no visibility into where the money goes. The CFO sees a lump-sum bill. Engineering has no tools to analyze spend by model, provider, or team. The existing solutions (LiteLLM, Portkey, Helicone, Langfuse) all require deployment, configuration, and commitment before you get any value. They're infrastructure you install, not tools you run.

## The Product

TokenSentinel is a single Go binary. Download it, upload your provider billing CSV, and get a professional PDF savings report in 30 seconds. No Docker, no Redis, no Postgres, no signup.

The prescriptive engine finds:
- **Model substitutions** — cheaper equivalent models that deliver the same quality
- **Provider switching** — when self-hosted or alternative providers would save money
- **Waste detection** — 3-sigma anomalies flag unusual spend patterns
- **GPU downsizing** — underutilized infrastructure that can be right-sized
- **Batch optimization** — workloads that could move to off-peak pricing

It auto-detects CSV formats from OpenAI, Anthropic, Google, Mistral, and more. Supports 20+ currencies with automatic FX rates. The entire analysis runs locally — no data ever leaves your machine.

## Traction

The product is built and working:
- Single Go binary, ~13MB, cross-platform (Windows/macOS/Linux)
- 37 passing tests, CI pipeline
- Deployable to Fly.io via Dockerfile
- Available on GitHub (github.com/Tejas163/TokenSentinel)

## Market

Every company using LLM APIs in production is a customer. The market for AI cost governance is growing with the LLM market itself. Early adopters are engineering leaders who've felt the pain of $50k+ monthly bills with no visibility into where the money went.

## The Ask

Raising a seed round to:
- Build the API key import layer (live billing data from every provider)
- Expand the prescriptive engine with real-time recommendations
- Add collaborative features (share reports, team benchmarks)
- Hire a founding engineer

---

**TokenSentinel** — github.com/Tejas163/TokenSentinel
