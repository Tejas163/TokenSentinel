from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime


@dataclass
class GPUConfig:
    type: str
    count: int
    region: str
    hourly_price: float
    reserved: bool = False


@dataclass
class TokenDistribution:
    input_pct: float = 0.7
    output_pct: float = 0.3


@dataclass
class ProviderUsed:
    name: str
    models: list[str]
    monthly_spend: float


@dataclass
class TeamComposition:
    developers: int = 0
    platform_engineers: int = 0
    devops: int = 0
    management: int = 0


@dataclass
class Assessment:
    id: Optional[int] = None
    company_name: str = ""
    cloud_vendor: str = "aws"
    gpu_configs: list[GPUConfig] = field(default_factory=list)
    monthly_request_volume: int = 0
    token_distribution: TokenDistribution = field(default_factory=TokenDistribution)
    current_monthly_spend: float = 0.0
    providers_used: list[ProviderUsed] = field(default_factory=list)
    team_composition: TeamComposition = field(default_factory=TeamComposition)
    source: str = "manual"
    version: int = 1
    created_at: Optional[str] = None
    updated_at: Optional[str] = None


@dataclass
class CostProjection:
    model: str
    provider: str
    current_monthly_cost: float
    projected_monthly_cost: float
    input_tokens_millions: float
    output_tokens_millions: float
    scenario: str = "base"
    assessment_id: Optional[int] = None
    id: Optional[int] = None


@dataclass
class Recommendation:
    category: str
    description: str
    current_cost: float
    projected_cost: float
    monthly_savings: float
    payback_period_days: int
    priority: str
    assessment_id: Optional[int] = None
    id: Optional[int] = None
    created_at: Optional[str] = None


@dataclass
class AssessmentReport:
    assessment: Assessment
    cost_breakdown: list[CostProjection]
    recommendations: list[Recommendation]
    total_current_spend: float
    total_projected_spend: float
    total_monthly_savings: float


@dataclass
class BudgetRule:
    id: Optional[int] = None
    name: str = ""
    provider: str = ""
    model: str = ""
    monthly_limit: float = 0.0
    alert_threshold: float = 0.8
    enabled: bool = True


@dataclass
class MonitoringRule:
    id: Optional[int] = None
    name: str = ""
    metric: str = ""
    condition: str = "above"
    threshold: float = 0.0
    cooldown_minutes: int = 30
    enabled: bool = True
    channels: list[str] = field(default_factory=lambda: ["email"])


@dataclass
class AnomalyAlert:
    id: int
    model: str
    current_cost: float
    expected_cost: float
    deviation_pct: float
    severity: str
    timestamp: str
    acknowledged: bool = False


@dataclass
class SavingsEvent:
    id: int
    model: str
    estimated_monthly_savings: float
    previous_monthly_cost: float
    current_monthly_cost: float
    detected_at: str


@dataclass
class CostEntry:
    id: int
    request_id: str
    model: str
    input_tokens: int
    output_tokens: int
    cost: float
    timestamp: str
    team: str = ""
