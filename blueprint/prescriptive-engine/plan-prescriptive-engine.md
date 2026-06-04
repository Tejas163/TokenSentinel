# Plan: Prescriptive Engine

## Overview

- TokenSentinel currently diagnoses what happened; the prescriptive engine tells users what to do and how much they'll save
- New module in `cost-dashboard` (Go) — no new service, reuses existing PostgreSQL, auth, and dashboard HTML patterns
- Accepts cloud vendor details, GPU config, model usage, team composition via web form or JSON/YAML upload
- Generates model-level cost breakdowns, waste analysis, model substitution recommendations, infra downsizing, batch optimization, and provider switching advice
- Report output: HTML dashboard page, PDF (via maroto), CSV export, with interactive What-If sliders
- Assessment lifecycle is versioned — each edit creates a new version with overlay comparison
- Pricing built on existing `cost_estimator.go` table + new GPU/cloud reference pricing (user-overridable)

## Expected behavior

- User navigates to `/prescriptive/assessments/new` on the cost-dashboard and sees a web form with fields: cloud vendor, GPU types/counts/pricing, monthly request volume, token distribution (input vs output), current monthly spend, providers and models used, team composition by role
- User can also upload a JSON or YAML file with the same schema instead of filling the form
- Three starter templates available: "Startup (5 devs, OpenAI only)", "Mid-size (20 devs, multi-model)", "Enterprise (50+ devs, self-hosted)"
- On submit, the engine either auto-pulls live cost data from `cost_entries` (if available and user opts in) or uses the user-provided spend figures
- Calculation runs: cost breakdown by model, waste analysis (unused GPU capacity, suboptimal model choices, cross-region costs), model substitution recommendations (capability-tier matrix with equivalence-group fallback), provider switching analysis, batch optimization opportunities
- Report page loads with sections: Executive Summary → Cost Breakdown → Recommendations → What-If Simulator → Appendices (data tables, CSV download)
- Executive summary shows total current spend, projected savings, payback period, top recommendation
- Cost breakdown shows per-model and per-provider spend with team drill-down toggle (aggregate view by default, click to expand per-team from `cost_entries.team`)
- Recommendations sorted by priority (high/medium/low), each with current cost, projected cost, monthly savings, payback period in days
- What-If section has sliders for request volume (±50%), model distribution (rebalance % across models), GPU cluster size, and provider pricing overrides — adjusts projections client-side via embedded JS
- PDF download button renders the full report via maroto (Go server-side), CSV download provides raw data tables
- User can edit an assessment, creating a new version; version comparison shows overlay/tabbed view between any two versions
- CSV import from any provider with user-defined column mapping
- All new endpoints require existing `X-Api-Key` auth (same as existing dashboard routes)

## Implementation plan

### New PostgreSQL tables (`cost-dashboard/main.go` schema init or new migration)

- `assessments` — company_name, cloud_vendor, gpu_types (JSONB: type, count, region, hourly_price, reserved), monthly_request_volume, token_distribution (JSONB: input_pct, output_pct), current_monthly_spend, providers_used (JSONB: name, models[], monthly_spend), team_composition (JSONB: role -> count), version, source (live|manual), created_at, updated_at
- `recommendations` — id, assessment_id (FK), category (model_switch|infra_downsize|batch_optimization|provider_switch), description, current_cost, projected_cost, monthly_savings, payback_period_days, priority (high|medium|low), created_at
- `cost_projections` — id, assessment_id (FK), model, provider, current_monthly_cost, projected_monthly_cost, input_tokens_millions, output_tokens_millions, scenario (base|whatif), created_at
- `assessment_versions` — id, assessment_id (FK), version_number, snapshot (JSONB of full input data), created_at

### New Go files in `cost-dashboard/`

- `prescriptive/models.go` — Go structs for assessment, recommendation, cost_projection, assessment_version; JSON tags matching the table schema
- `prescriptive/engine.go` — Core calculation engine:
  - `RunAssessment(input *Assessment) (*Report, error)` — orchestrates the full pipeline
  - `calculateCostBreakdown(input, liveData?)` — splits spend by model/provider, applies pricing table
  - `findWaste(input)` — identifies GPU underutilization, over-provisioned nodes, suboptimal model choices
  - `findModelSubstitutions(input, costBreakdown)` — capability-tier matrix (benchmark-ranked: frontier, capable, fast, cheap) with equivalence-group fallback; suggests cheaper alternatives per use case
  - `findInfraDownsizeOpportunities(input)` — GPU cluster rightsizing based on utilization assumptions
  - `findProviderSwitchOpportunities(input)` — cross-provider price comparison using reference pricing
  - `findBatchOptimizationOpportunities(input)` — identifies workloads suitable for off-peak/batch
  - `calculateROI(recommendations)` — total savings, payback period, projected monthly/quarterly/annual ROI
  - `RunWhatIf(assessmentID, adjustments)` — recalculates projections with modified params (used by slider API)
- `prescriptive/handlers.go` — HTTP handlers:
  - `GET /api/prescriptive/assessments` — list assessments (paginated)
  - `POST /api/prescriptive/assessments` — create assessment (accepts JSON body or multipart file upload)
  - `GET /api/prescriptive/assessments/{id}` — get assessment detail
  - `PUT /api/prescriptive/assessments/{id}` — update assessment (creates new version)
  - `DELETE /api/prescriptive/assessments/{id}` — delete assessment + all versions
  - `GET /api/prescriptive/assessments/{id}/versions` — list versions
  - `GET /api/prescriptive/assessments/{id}/versions/{version}` — get specific version snapshot
  - `POST /api/prescriptive/assessments/{id}/run` — run/re-run assessment engine
  - `GET /api/prescriptive/report/{id}` — HTML report page
  - `GET /api/prescriptive/report/{id}/pdf` — download PDF report (maroto-generated)
  - `GET /api/prescriptive/report/{id}/csv` — download CSV data tables
  - `POST /api/prescriptive/what-if/{id}` — What-If recalculation (accepts slider adjustments in body)
  - `POST /api/prescriptive/import/csv` — import provider billing CSV with column mapping
- `prescriptive/templates.go` — HTML template(s) for the report page (embedded similar to dashboard.html)
- `prescriptive/pdf.go` — PDF report generation using maroto (server-side, structured tables, sections matching report)
- `prescriptive/templates.go` or `prescriptive/import.go` — CSV import parser with user-defined column mapping logic
- `prescriptive/compare.go` — version comparison logic (overlay/tabbed data structure)

### Modified files

- `cost-dashboard/main.go` — register new routes, init new tables in DB setup, add prescriptive engine to goroutine lifecycle
- `cost-dashboard/Dockerfile` — add `maroto` and new dependencies to go.mod; no structural changes needed
- `proxyops_gateway/docker-compose.yml` — no changes needed (same service, new routes)
- `docs/` — add prescriptive engine section to API reference and user manual

## Implementation phases

### Phase 1: Data model + ingestion (working scaffold)
- Create `prescriptive/models.go` with Go structs and `prescriptive/handlers.go` with CRUD endpoints
- Add assessment, recommendation, cost_projection, version tables to DB schema init in main.go
- Implement form-based assessment creation and JSON/YAML upload
- Implement starter templates (3 hardcoded templates)
- Wire up auth middleware to new endpoints
- **Result:** User can create, list, update, delete assessments via API

### Phase 2: Calculation engine (core logic)
- Build `prescriptive/engine.go` with cost breakdown, waste analysis, model substitution (tier matrix), infra downsizing, provider switching, batch optimization
- Build ROI calculation
- Wire `POST /assessments/{id}/run` endpoint
- Integrate optional live data pull from cost_entries table
- **Result:** User can submit assessment inputs and get back a structured report JSON

### Phase 3: HTML report page + What-If (user-facing)
- Build `prescriptive/templates.go` with embedded HTML template (similar to dashboard.html pattern)
- Sections: Executive Summary → Cost Breakdown → Recommendations → What-If → Appendices
- Implement What-If sliders with client-side JS recalculation
- Implement team drill-down toggle
- Add version comparison overlay UI
- **Result:** Full interactive report in browser

### Phase 4: Export (PDF + CSV)
- Build `prescriptive/pdf.go` with maroto-based report layout: title page, exec summary table, cost breakdown table, recommendations table, ROI chart placeholder
- Build CSV export handler that dumps cost_projection and recommendation tables
- **Result:** PDF and CSV downloads from report page

### Phase 5: CSV import + polish
- Build CSV import handler with user-defined column mapping (form on frontend, parser on backend)
- Add three starter templates to the assessment form
- Add loading states, error handling, input validation to all forms
- Update docs with new endpoints and user manual section
- **Result:** Complete prescriptive feature

## Testing strategy

- **Unit tests** for `engine.go`: test cost breakdown with known inputs, test model substitution with tier matrix (verify it picks the right alternative), test waste detection with mock GPU configs, test ROI math
- **Unit tests** for `pdf.go`: verify maroto document generates without error (smoke test)
- **Unit tests** for `import.go`: test CSV parsing with OpenAI, Anthropic, and custom column mappings
- **Integration tests** for handlers: CRUD lifecycle (create → run → read report → update → verify version bump → compare versions)
- **Integration tests** for what-if: create assessment, run, apply slider adjustments, verify recalculated projections differ
- **Edge cases:** zero-token assessments, single-model setups, on-prem (no GPU cost), missing team data, very large request volumes, negative slider values
- **Manual test:** full workflow via the demo script (`demo/demo.ps1`)

## Open questions

- What benchmark scores should seed the capability-tier matrix? Can use HF Open LLM Leaderboard or implement as user-configurable weights.
- Should What-If slider changes auto-save as draft versions or only persist when user clicks "Save as version"?
- PDF localization / branding — should the report include company logo and custom branding?
- How many versions to retain per assessment? (Unlimited, or cap at N with oldest pruning.)
- Should the CSV import automatically detect provider from column headers, or always require explicit mapping?
