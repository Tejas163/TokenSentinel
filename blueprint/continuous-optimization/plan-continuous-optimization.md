# Plan: Continuous Optimization Engine

## Overview

- Phase 6 upgrades the prescriptive engine from a one-time report tool to a continuous background monitoring system
- New background goroutines in cost-dashboard (following existing `detectAnomalies`, `checkBudgets` pattern) run every 5 minutes
- Monitor spend trends per-model and per-provider, compare against rolling baselines, fire alerts when thresholds breached
- Auto-detect when recommendations have been implemented by watching cost/volume drops and usage pattern shifts
- Track actual savings over time and display in a timeline chart on the dashboard
- Extends existing budget rules — does NOT replace them
- Alert channels: signed webhooks (same pattern as existing budget webhooks), SMTP/SendGrid email, in-app dashboard toasts + banner list

## Expected behavior

- Background goroutines run every 5 minutes: `monitorSpendTrends` (spend increase alerts), `trackSavings` (recommendation implementation detection), `sendAlerts` (alert dispatch to all configured channels)
- When a model's spend increases >20% vs rolling 7-day average AND the absolute increase exceeds a configurable threshold, an alert event is created
- Alert thresholds are configurable per-model via `monitoring_rules` table (pct_threshold, abs_threshold, enabled, channels)
- Default thresholds apply to unconfigured models: 20% increase, $100 absolute
- Alert events are stored in `alerts` table with type, severity, model, message, created_at, acknowledged_at, dismissed_at
- In-app alerts appear as toast notifications (top of dashboard, auto-dismiss 30s) and in a persistent alert banner list
- Webhook alerts POST to configured URLs with HMAC-SHA256 signature (same `signAndPost` helper)
- Email alerts render a plain-text template and send via SMTP (with optional SendGrid/Mailgun upgrade path)
- Savings tracking detects when a model's monthly cost drops >30% compared to baseline (pre-recommendation average), or when usage pattern shifts away from recommended-against models
- Detected savings are stored in `savings_events` table: recommendation_id, model, detected_at, previous_monthly_cost, current_monthly_cost, estimated_monthly_savings, confidence (high for cost-drop, medium for pattern-shift)
- Dashboard shows a savings tracking section with summary table (model, rec date, past cost, current cost, savings) and a cost-over-time timeline chart with annotations for recommendation dates
- User can acknowledge/dismiss alerts via API/UI
- Alerts do NOT auto-create assessments — user decides when to re-run

## Implementation plan

### New PostgreSQL tables

- `monitoring_rules` — id, model (or `*` for all), pct_threshold (default 20), abs_threshold (default 100), period (default '7d'), enabled, webhook_url, email_to, created_at, updated_at
- `alerts` — id, monitoring_rule_id (nullable FK), model, alert_type (spend_spike|savings_opportunity|budget_exceeded), severity (info|warning|critical), message, current_value, threshold_value, metadata (JSONB), acknowledged_at, dismissed_at, created_at
- `savings_events` — id, assessment_id (nullable FK), recommendation_id (nullable), model, detected_at, detection_method (cost_drop|pattern_shift), previous_monthly_cost, current_monthly_cost, estimated_monthly_savings, confidence (high|medium), notes, created_at

### New Go files

- `monitoring_engine.go` — Background goroutines:
  - `monitorSpendTrends(ctx)` — every 5 min, queries cost_entries per model for rolling 7d window, compares to previous 7d window, creates alert if threshold breached
  - `trackSavings(ctx)` — every 5 min, checks cost_entries for recent cost drops >30% vs baseline, checks if any un-tracked recommendations match, creates savings_event
  - `sendAlerts(ctx)` — dispatches pending alerts to configured channels (webhook, email, SSE broadcast for in-app)
- `monitoring_handlers.go` — CRUD for monitoring_rules, list/ack/dismiss alerts, savings tracking data
  - `GET /api/monitoring/rules` — list rules
  - `POST /api/monitoring/rules` — create rule
  - `PUT /api/monitoring/rules/{id}` — update rule
  - `DELETE /api/monitoring/rules/{id}` — delete rule
  - `GET /api/monitoring/alerts` — list alerts (filterable: unacknowledged, type, model)
  - `POST /api/monitoring/alerts/{id}/acknowledge` — mark alert acknowledged
  - `POST /api/monitoring/alerts/{id}/dismiss` — dismiss alert
  - `GET /api/monitoring/savings` — savings tracking data
  - `GET /api/monitoring/trends/{model}` — cost time-series data for timeline chart
- `monitoring_email.go` — Email sending:
  - SMTP sender (config: SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, FROM_ADDR)
  - Optional SendGrid/Mailgun integration (config: SENDGRID_API_KEY)
  - `sendEmailAlert(alert)` — renders text template and sends
- `monitoring_notify.go` — Notification dispatch:
  - Core dispatch function that fans out to webhook, email, SSE
  - Webhook: reuses existing `signAndPost` helper
  - SSE: reuses existing `events.broad` channel
  - Email: calls `monitoring_email.sendEmailAlert`
- `prescriptive_savings.go` — Savings tracking dashboard data:
  - Queries savings_events + recommendations + cost_entries for dashboard rendering
  - Builds time-series data for cost trend charts

### Modified files

- `main.go` — register new monitoring routes + start monitoring goroutines
- `prescriptive_handlers.go` — add monitoring routes to router dispatch
- `prescriptive_export.go` — extend export to include savings tracking data
- `report.html` — add alert banner section and savings tracking panel
- `dashboard.html` — add alert toast/banner overlay (separate from prescriptive report page)
- `docs/api-reference.html` — add monitoring endpoints
- `USER_MANUAL.md` — add Continuous Optimization section

## Implementation phases

### Phase 6a: Monitoring rules + spend trend alerts
- Create `monitoring_rules` and `alerts` tables
- Build `monitoring_engine.go` with `monitorSpendTrends` goroutine
- Build `monitoring_handlers.go` with CRUD for rules + alert list/ack/dismiss
- Wire routes and start goroutine in main.go
- **Result:** Engine detects spend spikes, stores alerts, user can list/ack alerts via API

### Phase 6b: Alert dispatch (webhook + SSE + email)
- Build `monitoring_notify.go` for webhook + SSE dispatch
- Build `monitoring_email.go` for SMTP/SendGrid email sending
- Wire dispatch into `sendAlerts` goroutine (runs every 30s, dispatches pending unacknowledged alerts)
- **Result:** Alerts flow through all channels — webhook POST, SSE broadcast to dashboard, email

### Phase 6c: Savings auto-detection + tracking dashboard
- Build `trackSavings` goroutine — detects cost drops and pattern shifts
- Build `prescriptive_savings.go` — time-series data for trends
- Add savings tracking panel to report.html (summary table + timeline chart)
- **Result:** Savings auto-detected and displayed with cost trend chart

### Phase 6d: Dashboard alert UI + polish
- Add toast notification overlay to dashboard.html (listens to SSE for alert events)
- Add persistent alert banner list panel
- Add acknowledgment buttons
- Update docs with monitoring endpoints and user guide
- **Result:** Complete continuous optimization system

## Testing strategy

- Unit tests for `monitorSpendTrends`: mock cost_entries with known spikes, verify alert created when threshold breached, verify no alert when under threshold
- Unit tests for `trackSavings`: mock cost_entries with known drops, verify savings_event created with correct confidence
- Unit tests for alert dispatch: verify webhook POST called, verify SSE event broadcast, verify email sent
- Integration tests: create monitoring rule, insert cost data with spike, run monitor goroutine (triggered manually), verify alert created and dispatchable
- Edge cases: zero-cost models, models with no prior data, threshold at boundary values, email delivery failure

## Open questions

- Should savings events auto-link to a prescriptive recommendation, or standalone?
- Email template design — plain text vs HTML?
- Cost trend chart — simple inline SVG or canvas-based rendering?
- Alert de-duplication — how long before the same condition re-alerts? (30-min cooldown like budget rules, or different?)
- Should alerts auto-escalate (warning → critical) if unacknowledged for >24h?
