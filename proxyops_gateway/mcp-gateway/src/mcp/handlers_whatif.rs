use crate::prescriptive::client;

pub async fn handle_whatif_multi_scenario(
    base_assessment_id: i64,
    scenarios: Vec<serde_json::Value>,
    team: Option<&str>,
) -> Result<serde_json::Value, String> {
    let mut results = Vec::new();
    for (i, scenario) in scenarios.into_iter().enumerate() {
        let result = client::run_whatif(base_assessment_id, &scenario, team).await
            .unwrap_or(serde_json::json!({"error": format!("scenario {i} failed")}));
        results.push(serde_json::json!({
            "scenario_index": i,
            "adjustments": scenario,
            "projection": result,
        }));
    }
    Ok(serde_json::json!({
        "base_assessment_id": base_assessment_id,
        "scenario_count": results.len(),
        "scenarios": results,
    }))
}

pub async fn handle_whatif_volume_shift(
    assessment_id: i64,
    volume_pct: f64,
    team: Option<&str>,
) -> Result<serde_json::Value, String> {
    let adjustment = serde_json::json!({
        "request_volume_pct_change": volume_pct,
    });
    client::run_whatif(assessment_id, &adjustment, team).await
}

pub async fn handle_whatif_model_switch(
    assessment_id: i64,
    from_model: &str,
    to_model: &str,
    team: Option<&str>,
) -> Result<serde_json::Value, String> {
    let adjustment = serde_json::json!({
        "model_switch": {
            "from": from_model,
            "to": to_model,
        }
    });
    client::run_whatif(assessment_id, &adjustment, team).await
}
