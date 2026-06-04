# TokenSentinel Python SDK

Official Python client for the [TokenSentinel](https://github.com/Tejas163/TokenSentinel) LLM cost governance API.

## Installation

```bash
pip install tokensentinel
```

Or install from source:

```bash
cd sdk/python
pip install .
```

## Quick Start

```python
from tokensentinel import TokenSentinel, ProviderUsed, GPUConfig, TeamComposition

client = TokenSentinel(
    base_url="http://localhost:3001",
    api_key="your-api-key-here",
)

# Health check
print(client.health_check())

# Create an assessment
assessment = client.create_assessment(
    company_name="MyCompany",
    monthly_spend=12000,
    monthly_request_volume=500000,
    cloud_vendor="aws",
    providers=[
        ProviderUsed(name="openai", models=["gpt-4o", "gpt-4o-mini"], monthly_spend=7000),
        ProviderUsed(name="anthropic", models=["claude-3-sonnet"], monthly_spend=5000),
    ],
    gpu_configs=[
        GPUConfig(type="A100", count=4, region="us-east-1", hourly_price=3.50, reserved=True),
    ],
    team=TeamComposition(developers=20, platform_engineers=2, devops=1, management=2),
)
print(f"Created assessment #{assessment.id}")

# Run the prescriptive engine
report = client.run_assessment(assessment.id)
print(f"Current spend: ${report.total_current_spend:,.2f}")
print(f"Projected savings: ${report.total_monthly_savings:,.2f}")

for rec in report.recommendations:
    print(f"  [{rec.priority}] {rec.description} — save ${rec.monthly_savings:.0f}/mo")

# What-if simulation
projections = client.run_what_if(assessment.id, volume_multiplier=1.5)
for p in projections:
    print(f"  {p.model}: ${p.current_monthly_cost:.2f} → ${p.projected_monthly_cost:.2f}")
```

## API Reference

### Client

| Method | Description |
|--------|-------------|
| `health_check()` | Health check endpoint |
| `list_assessments()` | List all assessments |
| `get_assessment(id)` | Get a single assessment |
| `create_assessment(...)` | Create a new assessment |
| `update_assessment(id, **kwargs)` | Update an assessment |
| `delete_assessment(id)` | Delete an assessment |
| `get_templates()` | Get starter templates |
| `run_assessment(id)` | Execute prescriptive engine |
| `get_report(id)` | Get assessment report |
| `run_what_if(id, ...)` | Run what-if simulation |
| `get_report_csv(id)` | Download CSV report |
| `get_report_pdf(id)` | Download PDF report |
| `get_dashboard_summary()` | Get live cost summary |
| `get_dashboard_costs()` | Get live cost entries |
| `get_anomalies()` | Get anomaly alerts |
| `list_budget_rules()` | List budget rules |
| `create_budget_rule(...)` | Create budget rule |
| `get_budget_status()` | Get budget status |
| `list_teams()` | List teams |
| `create_team(...)` | Create team |
| `list_monitoring_rules()` | List monitoring rules |
| `create_monitoring_rule(...)` | Create monitoring rule |
| `get_savings_events()` | Get savings events |

## Requirements

- Python >= 3.10
- No external dependencies (uses stdlib `urllib`)
