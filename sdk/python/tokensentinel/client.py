"""TokenSentinel API client.

Usage:
    from tokensentinel import TokenSentinel

    client = TokenSentinel(base_url="http://localhost:3001", api_key="dev-key-123")

    # Create an assessment
    a = client.create_assessment(company_name="MyCo", monthly_spend=5000, ...)

    # Run the prescriptive engine
    report = client.run_assessment(a.id)

    # View recommendations
    for rec in report.recommendations:
        print(rec.description, rec.monthly_savings)
"""

import json
import urllib.request
import urllib.error
from typing import Optional
from .models import (
    Assessment, AssessmentReport, CostProjection, Recommendation,
    GPUConfig, TokenDistribution, ProviderUsed, TeamComposition,
    BudgetRule, MonitoringRule, AnomalyAlert, SavingsEvent, CostEntry,
)


class TokenSentinelError(Exception):
    pass


class TokenSentinel:
    def __init__(self, base_url: str, api_key: str, timeout: int = 30):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout

    def _headers(self) -> dict[str, str]:
        return {
            "Content-Type": "application/json",
            "X-Api-Key": self.api_key,
        }

    def _request(self, method: str, path: str, body: Optional[dict] = None) -> dict | list:
        url = f"{self.base_url}{path}"
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, headers=self._headers(), method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            msg = e.read().decode() if e.fp else str(e)
            raise TokenSentinelError(f"HTTP {e.code}: {msg}")
        except urllib.error.URLError as e:
            raise TokenSentinelError(f"Connection failed: {e.reason}")

    # ------------------------------------------------------------------ #
    #  Assessments
    # ------------------------------------------------------------------ #

    def list_assessments(self) -> list[Assessment]:
        data = self._request("GET", "/api/prescriptive/assessments")
        return [Assessment(**a) for a in data]

    def get_assessment(self, assessment_id: int) -> Assessment:
        data = self._request("GET", f"/api/prescriptive/assessments/{assessment_id}")
        return Assessment(**Assessment._asdict(data))

    def create_assessment(
        self,
        company_name: str,
        monthly_spend: float,
        monthly_request_volume: int,
        cloud_vendor: str = "aws",
        providers: Optional[list[ProviderUsed]] = None,
        gpu_configs: Optional[list[GPUConfig]] = None,
        token_distribution: Optional[TokenDistribution] = None,
        team: Optional[TeamComposition] = None,
        source: str = "manual",
    ) -> Assessment:
        payload = {
            "company_name": company_name,
            "cloud_vendor": cloud_vendor,
            "current_monthly_spend": monthly_spend,
            "monthly_request_volume": monthly_request_volume,
            "providers_used": [
                {"name": p.name, "models": p.models, "monthly_spend": p.monthly_spend}
                for p in (providers or [])
            ],
            "gpu_configs": [
                {"type": g.type, "count": g.count, "region": g.region,
                 "hourly_price": g.hourly_price, "reserved": g.reserved}
                for g in (gpu_configs or [])
            ],
            "token_distribution": {
                "input_pct": (token_distribution or TokenDistribution()).input_pct,
                "output_pct": (token_distribution or TokenDistribution()).output_pct,
            },
            "team_composition": {
                "developers": (team or TeamComposition()).developers,
                "platform_engineers": (team or TeamComposition()).platform_engineers,
                "devops": (team or TeamComposition()).devops,
                "management": (team or TeamComposition()).management,
            },
            "source": source,
        }
        data = self._request("POST", "/api/prescriptive/assessments", payload)
        return Assessment(**data)

    def update_assessment(self, assessment_id: int, **kwargs) -> Assessment:
        data = self._request("PUT", f"/api/prescriptive/assessments/{assessment_id}", kwargs)
        return Assessment(**data)

    def delete_assessment(self, assessment_id: int) -> None:
        self._request("DELETE", f"/api/prescriptive/assessments/{assessment_id}")

    def get_templates(self) -> list[dict]:
        return self._request("GET", "/api/prescriptive/templates")

    # ------------------------------------------------------------------ #
    #  Prescriptive Engine
    # ------------------------------------------------------------------ #

    def run_assessment(self, assessment_id: int) -> AssessmentReport:
        data = self._request("POST", f"/api/prescriptive/assessments/{assessment_id}/run")
        return self._parse_report(data)

    def get_report(self, assessment_id: int) -> AssessmentReport:
        data = self._request("GET", f"/api/prescriptive/report/{assessment_id}")
        return self._parse_report(data)

    def _parse_report(self, data: dict) -> AssessmentReport:
        assessment = Assessment(**data.get("assessment", {}))
        cost_breakdown = [CostProjection(**c) for c in data.get("cost_breakdown", [])]
        recommendations = [Recommendation(**r) for r in data.get("recommendations", [])]
        return AssessmentReport(
            assessment=assessment,
            cost_breakdown=cost_breakdown,
            recommendations=recommendations,
            total_current_spend=data.get("total_current_spend", 0),
            total_projected_spend=data.get("total_projected_spend", 0),
            total_monthly_savings=data.get("total_monthly_savings", 0),
        )

    def run_what_if(self, assessment_id: int, volume_multiplier: float = 1.0,
                    input_pct: Optional[float] = None) -> list[CostProjection]:
        payload = {"volume_multiplier": volume_multiplier}
        if input_pct is not None:
            payload["input_pct"] = input_pct
        data = self._request("POST", f"/api/prescriptive/what-if/{assessment_id}", payload)
        return [CostProjection(**c) for c in data]

    def get_report_csv(self, assessment_id: int) -> str:
        return self._request_raw("GET", f"/api/prescriptive/report/{assessment_id}/csv")

    def get_report_pdf(self, assessment_id: int) -> bytes:
        return self._request_raw_bytes("GET", f"/api/prescriptive/report/{assessment_id}/pdf")

    def import_csv(self, assessment_id: int, csv_content: str,
                   model_column: int = 0, input_column: int = 1,
                   output_column: int = 2, cost_column: int = 3) -> dict:
        payload = {
            "assessment_id": assessment_id,
            "model_column": model_column,
            "input_tokens_column": input_column,
            "output_tokens_column": output_column,
            "cost_column": cost_column,
            "has_header": True,
        }
        return self._request("POST", f"/api/prescriptive/import/csv?body={csv_content}", payload)

    def get_versions(self, assessment_id: int) -> list[dict]:
        return self._request("GET", f"/api/prescriptive/assessments/{assessment_id}/versions")

    # ------------------------------------------------------------------ #
    #  Dashboard / Cost Data
    # ------------------------------------------------------------------ #

    def get_dashboard_summary(self) -> dict:
        return self._request("GET", "/api/dashboard/summary")

    def get_dashboard_costs(self) -> list[CostEntry]:
        data = self._request("GET", "/api/dashboard/costs")
        return [CostEntry(**c) for c in data]

    def get_anomalies(self) -> list[AnomalyAlert]:
        data = self._request("GET", "/api/dashboard/anomalies")
        return [AnomalyAlert(**a) for a in data]

    # ------------------------------------------------------------------ #
    #  Budget Rules
    # ------------------------------------------------------------------ #

    def list_budget_rules(self) -> list[BudgetRule]:
        data = self._request("GET", "/api/admin/budget-rules")
        return [BudgetRule(**r) for r in data]

    def create_budget_rule(self, name: str, monthly_limit: float,
                           provider: str = "", model: str = "",
                           alert_threshold: float = 0.8) -> BudgetRule:
        payload = {
            "name": name,
            "provider": provider,
            "model": model,
            "monthly_limit": monthly_limit,
            "alert_threshold": alert_threshold,
        }
        data = self._request("POST", "/api/admin/budget-rules", payload)
        return BudgetRule(**data)

    def get_budget_status(self) -> list[dict]:
        return self._request("GET", "/api/budget/status")

    # ------------------------------------------------------------------ #
    #  Teams
    # ------------------------------------------------------------------ #

    def list_teams(self) -> list[dict]:
        return self._request("GET", "/api/admin/teams")

    def create_team(self, name: str, monthly_budget: float = 0) -> dict:
        return self._request("POST", "/api/admin/teams", {"name": name, "monthly_budget": monthly_budget})

    # ------------------------------------------------------------------ #
    #  Monitoring
    # ------------------------------------------------------------------ #

    def list_monitoring_rules(self) -> list[MonitoringRule]:
        data = self._request("GET", "/api/monitoring/rules")
        return [MonitoringRule(**r) for r in data]

    def create_monitoring_rule(self, name: str, metric: str, condition: str,
                                threshold: float, cooldown_minutes: int = 30,
                                channels: Optional[list[str]] = None) -> MonitoringRule:
        payload = {
            "name": name,
            "metric": metric,
            "condition": condition,
            "threshold": threshold,
            "cooldown_minutes": cooldown_minutes,
            "channels": channels or ["email"],
        }
        data = self._request("POST", "/api/monitoring/rules", payload)
        return MonitoringRule(**data)

    def get_savings_events(self) -> list[SavingsEvent]:
        data = self._request("GET", "/api/monitoring/savings")
        return [SavingsEvent(**s) for s in data]

    def health_check(self) -> dict:
        return self._request("GET", "/health")

    def _request_raw(self, method: str, path: str) -> str:
        url = f"{self.base_url}{path}"
        req = urllib.request.Request(url, headers=self._headers(), method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return resp.read().decode()
        except urllib.error.HTTPError as e:
            raise TokenSentinelError(f"HTTP {e.code}: {e.fp.read().decode() if e.fp else str(e)}")

    def _request_raw_bytes(self, method: str, path: str) -> bytes:
        url = f"{self.base_url}{path}"
        req = urllib.request.Request(url, headers=self._headers(), method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return resp.read()
        except urllib.error.HTTPError as e:
            raise TokenSentinelError(f"HTTP {e.code}: {e.fp.read() if e.fp else str(e)}")
