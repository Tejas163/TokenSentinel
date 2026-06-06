use crate::prescriptive::client as prescriptive_client;

pub async fn handle_get_cost_summary(period: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::get_cost_summary(period, team).await
}

pub async fn handle_get_model_costs(period: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::get_model_costs(period, team).await
}

pub async fn handle_get_anomalies(period: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::get_anomalies(period, team).await
}

pub async fn handle_run_assessment(assessment_id: i64, team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::run_assessment(assessment_id, team).await
}

pub async fn handle_run_whatif(assessment_id: i64, adjustments: serde_json::Value, team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::run_whatif(assessment_id, &adjustments, team).await
}

pub async fn handle_get_budget_status(team: &str) -> Result<serde_json::Value, String> {
    prescriptive_client::get_budget_status(team).await
}

pub async fn handle_list_budget_rules(team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::list_budget_rules_with_team(team).await
}

pub async fn handle_get_report(assessment_id: i64, team: Option<&str>) -> Result<serde_json::Value, String> {
    prescriptive_client::get_report(assessment_id, team).await
}

pub async fn handle_get_model_catalog() -> Result<serde_json::Value, String> {
    prescriptive_client::get_model_catalog().await
}
