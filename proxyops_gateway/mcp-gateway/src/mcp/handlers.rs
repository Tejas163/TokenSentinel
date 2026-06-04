// MCP tool call handlers — dispatches tools/call to the appropriate backend
// Each handler maps to a TokenSentinel API endpoint or prescriptive engine function

use crate::prescriptive::client as prescriptive_client;

pub async fn handle_get_cost_summary(period: &str) -> Result<serde_json::Value, String> {
    prescriptive_client::get_cost_summary(period).await
}

pub async fn handle_get_model_costs(period: &str) -> Result<serde_json::Value, String> {
    prescriptive_client::get_model_costs(period).await
}

pub async fn handle_get_anomalies(period: &str) -> Result<serde_json::Value, String> {
    prescriptive_client::get_anomalies(period).await
}

pub async fn handle_run_assessment(assessment_id: i64) -> Result<serde_json::Value, String> {
    prescriptive_client::run_assessment(assessment_id).await
}

pub async fn handle_run_whatif(assessment_id: i64, adjustments: serde_json::Value) -> Result<serde_json::Value, String> {
    prescriptive_client::run_whatif(assessment_id, &adjustments).await
}

pub async fn handle_get_budget_status(team: &str) -> Result<serde_json::Value, String> {
    prescriptive_client::get_budget_status(team).await
}

pub async fn handle_list_budget_rules() -> Result<serde_json::Value, String> {
    prescriptive_client::list_budget_rules().await
}

pub async fn handle_get_report(assessment_id: i64) -> Result<serde_json::Value, String> {
    prescriptive_client::get_report(assessment_id).await
}
