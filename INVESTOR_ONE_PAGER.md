# TokenSentinel — One-Pager

**An open-source AI gateway that governs every token, every route, and every cost.**

---

## The Problem

AI spend is the fastest-growing line item in every engineering budget, and nobody has control.

Companies point their apps at OpenAI, Anthropic, and a dozen other providers with no visibility into who's spending what, on which model, for which team. The CFO sees a lump-sum bill. Engineering has no tools to enforce budgets. Teams accidentally burn through $10k in a weekend on long-context prompts.

The existing solutions? Cloud provider dashboards that show aggregate numbers hours late. Spreadsheets. Manual approval gates. Most teams give up and either block AI entirely or let costs run uncontrolled.

## The Product

TokenSentinel is an AI gateway that sits between your app and every LLM provider. One deploy and you get:

- **Cost tracking on every request** — Model, tokens, team, timestamp. Live dashboard with period filtering and per-model breakdowns.
- **Budget enforcement** — Per-team monthly token budgets. When a team exceeds its limit, the router automatically shifts them to the cheapest available provider instead of blocking them.
- **Intelligent routing** — Weighted load balancing across providers. Autonomous model selection (cheap vs capable based on prompt complexity). Circuit breakers and retry with backoff.
- **Anomaly detection** — 3-sigma alerts when a request burns far more tokens than normal for its model. Pushed to the dashboard in real-time via SSE.
- **Enterprise SDK** — Spring Boot starter with auto-configured clients, Micrometer metrics, and typed Java APIs.

The architecture is deliberately polyglot — Rust at the edge for performance, Go in the router for concurrency, Erlang for reliable telemetry, Python for prompt optimization. Each service speaks the language best suited to its job.

## Traction

The system is fully built and verified end-to-end:

- 5 services (Rust, Go x2, Erlang, Java SDK) running in Docker Compose or Kubernetes
- Live cost dashboard with SSE streaming, 3-sigma anomaly detection, Slack/Teams digest webhooks
- Budget-aware routing that degrades gracefully instead of failing
- CI pipeline, E2E tests, k6 benchmarks, K8s manifests
- 31+ tests passing

## Business Model

Open-source core (Apache 2.0) with paid enterprise tiers:

| Tier | Price | What's included |
|------|-------|----------------|
| **Community** | Free | Full gateway, dashboard, SDK, community support |
| **Team** | $999/mo | SSO/SAML, audit logs, priority support, custom rate limits |
| **Enterprise** | Custom | Dedicated support, on-prem deployment, SLA, custom policies |

## Market

Every company using LLMs in production is a customer. The TAM is the entire enterprise AI infrastructure layer — projected $13B+ by 2027. Early adopters are engineering teams at mid-to-large companies who've felt the pain of uncontrolled AI spend.

## Why Now

The market is flooding with new LLM providers. Companies want multi-provider strategies but lack the infrastructure to manage them. The first gateway that's open-source, production-hardened, and enterprise-ready will win developer mindshare and become the default choice — the nginx or Envoy for AI traffic.

## The Ask

Raising a seed round to:
- Build the managed cloud offering (hosted gateway, zero-devops deploy)
- Add SSO, audit trails, role-based access control
- Hire a founding engineer and a developer advocate
- Compete with the closed-source alternatives (Portkey, Helicone) on openness and price

---

**TokenSentinel** — github.com/Tejas163/TokenSentinel
