// MCP tool definitions — maps TokenSentinel capabilities to MCP tools/list schema
// 8 initial tools exposed to AI agents

use serde::Serialize;

#[derive(Debug, Serialize)]
pub struct McpTool {
    pub name: String,
    pub description: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input_schema: Option<serde_json::Value>,
}

pub fn list_tools() -> Vec<McpTool> {
    vec![
        McpTool {
            name: "get_cost_summary".into(),
            description: "Get aggregate token cost summary for a time period (1h, 6h, 24h, 72h, 168h)".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "period": { "type": "string", "enum": ["1h","6h","24h","72h","168h"] }
                },
                "required": ["period"]
            })),
        },
        McpTool {
            name: "get_model_costs".into(),
            description: "Get per-model token cost breakdown for a time period".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "period": { "type": "string", "enum": ["1h","6h","24h","72h","168h"] }
                },
                "required": ["period"]
            })),
        },
        McpTool {
            name: "get_anomalies".into(),
            description: "Detect anomalous token usage using 3-sigma outlier detection".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "period": { "type": "string", "enum": ["1h","6h","24h","72h","168h"] }
                },
                "required": ["period"]
            })),
        },
        McpTool {
            name: "run_assessment".into(),
            description: "Run a full cost assessment with model substitution, infra downsizing, and provider switch recommendations".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "assessment_id": { "type": "integer" }
                },
                "required": ["assessment_id"]
            })),
        },
        McpTool {
            name: "run_whatif".into(),
            description: "Run a what-if scenario with adjusted parameters (volume multiplier, input/output ratio)".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "assessment_id": { "type": "integer" },
                    "adjustments": { "type": "object" }
                },
                "required": ["assessment_id", "adjustments"]
            })),
        },
        McpTool {
            name: "get_budget_status".into(),
            description: "Check whether a team is over or under its monthly token budget".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "team": { "type": "string" }
                },
                "required": ["team"]
            })),
        },
        McpTool {
            name: "list_budget_rules".into(),
            description: "List all budget threshold alert rules with model, max tokens, period, and webhook URL".into(),
            input_schema: None,
        },
        McpTool {
            name: "get_report".into(),
            description: "Get a complete assessment report with cost breakdown and recommendations".into(),
            input_schema: Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "assessment_id": { "type": "integer" }
                },
                "required": ["assessment_id"]
            })),
        },
    ]
}
