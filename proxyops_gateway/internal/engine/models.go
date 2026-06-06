package engine

import "strings"

type GPUReference struct {
	Type         string  `json:"type"`
	Description  string  `json:"description"`
	HourlyPrice  float64 `json:"hourly_price"`
	MonthlyPrice float64 `json:"monthly_price"`
	Tier         string  `json:"tier"`
}

var GPUReferencePricing = []GPUReference{
	{Type: "A100", Description: "NVIDIA A100 80GB PCIe", HourlyPrice: 3.50, MonthlyPrice: 2520, Tier: "datacenter"},
	{Type: "H100", Description: "NVIDIA H100 80GB SXM", HourlyPrice: 4.50, MonthlyPrice: 3240, Tier: "datacenter"},
	{Type: "H200", Description: "NVIDIA H200 141GB SXM", HourlyPrice: 5.00, MonthlyPrice: 3600, Tier: "datacenter"},
	{Type: "B100", Description: "NVIDIA B100 192GB", HourlyPrice: 6.00, MonthlyPrice: 4320, Tier: "datacenter"},
	{Type: "L40S", Description: "NVIDIA L40S 48GB", HourlyPrice: 2.50, MonthlyPrice: 1800, Tier: "datacenter"},
	{Type: "RTX4090", Description: "NVIDIA RTX 4090 24GB", HourlyPrice: 0.50, MonthlyPrice: 360, Tier: "prosumer"},
	{Type: "RTX5090", Description: "NVIDIA RTX 5090 32GB", HourlyPrice: 0.75, MonthlyPrice: 540, Tier: "prosumer"},
}

type ModelTier int

const (
	TierFrontier ModelTier = iota + 1
	TierCapable
	TierFast
	TierCheap
)

type ModelInfo struct {
	Name        string
	Provider    string
	Tier        ModelTier
	InputPrice  float64
	OutputPrice float64
}

var ModelCatalog = []ModelInfo{
	{Name: "gpt-4", Provider: "openai", Tier: TierFrontier, InputPrice: 30.00, OutputPrice: 60.00},
	{Name: "gpt-4-turbo", Provider: "openai", Tier: TierFrontier, InputPrice: 10.00, OutputPrice: 30.00},
	{Name: "gpt-4o", Provider: "openai", Tier: TierCapable, InputPrice: 2.50, OutputPrice: 10.00},
	{Name: "gpt-4o-mini", Provider: "openai", Tier: TierFast, InputPrice: 0.15, OutputPrice: 0.60},
	{Name: "gpt-3.5-turbo", Provider: "openai", Tier: TierCheap, InputPrice: 0.50, OutputPrice: 1.50},
	{Name: "claude-3-opus", Provider: "anthropic", Tier: TierFrontier, InputPrice: 15.00, OutputPrice: 75.00},
	{Name: "claude-3-sonnet", Provider: "anthropic", Tier: TierCapable, InputPrice: 3.00, OutputPrice: 15.00},
	{Name: "claude-3-haiku", Provider: "anthropic", Tier: TierFast, InputPrice: 0.25, OutputPrice: 1.25},
	{Name: "gemini-1.5-pro", Provider: "google", Tier: TierFrontier, InputPrice: 1.25, OutputPrice: 5.00},
	{Name: "gemini-1.5-flash", Provider: "google", Tier: TierCapable, InputPrice: 0.075, OutputPrice: 0.30},
	{Name: "mistral-large", Provider: "mistral", Tier: TierFrontier, InputPrice: 2.00, OutputPrice: 6.00},
	{Name: "mistral-small", Provider: "mistral", Tier: TierFast, InputPrice: 0.60, OutputPrice: 1.80},
	{Name: "llama-3-70b", Provider: "self-hosted", Tier: TierCapable, InputPrice: 0.59, OutputPrice: 0.79},
	{Name: "llama-3-8b", Provider: "self-hosted", Tier: TierCheap, InputPrice: 0.05, OutputPrice: 0.20},
	{Name: "mixtral-8x7b", Provider: "self-hosted", Tier: TierCapable, InputPrice: 0.24, OutputPrice: 0.72},
}

var ModelEquivalence = map[string][]string{
	"gpt-4":         {"claude-3-opus", "gemini-1.5-pro", "mistral-large"},
	"gpt-4-turbo":   {"claude-3-opus", "gemini-1.5-pro"},
	"gpt-4o":        {"claude-3-sonnet", "gemini-1.5-flash", "llama-3-70b", "mixtral-8x7b"},
	"gpt-4o-mini":   {"claude-3-haiku", "mistral-small"},
	"gpt-3.5-turbo": {"llama-3-8b", "claude-3-haiku"},
	"claude-3-opus": {"gpt-4", "gemini-1.5-pro", "mistral-large"},
	"claude-3-sonnet": {"gpt-4o", "gemini-1.5-flash", "mixtral-8x7b"},
	"claude-3-haiku": {"gpt-4o-mini", "mistral-small"},
	"gemini-1.5-pro": {"gpt-4", "claude-3-opus", "mistral-large"},
	"gemini-1.5-flash": {"gpt-4o", "claude-3-sonnet"},
	"mistral-large": {"gpt-4", "claude-3-opus", "gemini-1.5-pro"},
	"mistral-small": {"gpt-4o-mini", "claude-3-haiku"},
	"llama-3-70b": {"gpt-4o", "mixtral-8x7b"},
	"llama-3-8b": {"gpt-3.5-turbo"},
}

type GPUConfig struct {
	Type        string  `json:"type"`
	Count       int     `json:"count"`
	Region      string  `json:"region"`
	HourlyPrice float64 `json:"hourly_price"`
	Reserved    bool    `json:"reserved"`
}

type TokenDistribution struct {
	InputPct  float64 `json:"input_pct"`
	OutputPct float64 `json:"output_pct"`
}

type ProviderUsage struct {
	Name         string   `json:"name"`
	Models       []string `json:"models"`
	MonthlySpend float64  `json:"monthly_spend"`
}

type TeamComposition struct {
	Developers        int `json:"developers"`
	PlatformEngineers int `json:"platform_engineers"`
	DevOps            int `json:"devops"`
	Management        int `json:"management"`
}

type Assessment struct {
	ID                   int               `json:"id"`
	OrgID                string            `json:"-"`
	CompanyName          string            `json:"company_name"`
	CloudVendor          string            `json:"cloud_vendor"`
	GPUConfigs           []GPUConfig       `json:"gpu_configs"`
	MonthlyRequestVolume int64             `json:"monthly_request_volume"`
	TokenDistribution    TokenDistribution `json:"token_distribution"`
	CurrentMonthlySpend  float64           `json:"current_monthly_spend"`
	ProvidersUsed        []ProviderUsage   `json:"providers_used"`
	TeamComposition      TeamComposition   `json:"team_composition"`
	Source               string            `json:"source"`
	Version              int               `json:"version"`
	CreatedAt            string            `json:"created_at"`
	UpdatedAt            string            `json:"updated_at"`
}

type Recommendation struct {
	ID                int     `json:"id"`
	AssessmentID      int     `json:"assessment_id"`
	Category          string  `json:"category"`
	Description       string  `json:"description"`
	CurrentCost       float64 `json:"current_cost"`
	ProjectedCost     float64 `json:"projected_cost"`
	MonthlySavings    float64 `json:"monthly_savings"`
	PaybackPeriodDays int     `json:"payback_period_days"`
	Priority          string  `json:"priority"`
	CreatedAt         string  `json:"created_at"`
}

type CostProjection struct {
	ID                   int     `json:"id"`
	AssessmentID         int     `json:"assessment_id"`
	Model                string  `json:"model"`
	Provider             string  `json:"provider"`
	CurrentMonthlyCost   float64 `json:"current_monthly_cost"`
	ProjectedMonthlyCost float64 `json:"projected_monthly_cost"`
	InputTokensMillions  float64 `json:"input_tokens_millions"`
	OutputTokensMillions float64 `json:"output_tokens_millions"`
	Scenario             string  `json:"scenario"`
	CreatedAt            string  `json:"created_at"`
}

type AssessmentReport struct {
	Assessment      Assessment       `json:"assessment"`
	CostBreakdown   []CostProjection `json:"cost_breakdown"`
	Recommendations []Recommendation  `json:"recommendations"`
	TotalCurrent    float64          `json:"total_current_spend"`
	TotalProjected  float64          `json:"total_projected_spend"`
	TotalSavings    float64          `json:"total_monthly_savings"`
}

func FindModel(name string) *ModelInfo {
	for i, m := range ModelCatalog {
		if m.Name == name {
			return &ModelCatalog[i]
		}
	}
	return nil
}

func FindGPUReference(gpuType string) *GPUReference {
	for i, g := range GPUReferencePricing {
		if strings.EqualFold(g.Type, gpuType) {
			return &GPUReferencePricing[i]
		}
	}
	return nil
}
