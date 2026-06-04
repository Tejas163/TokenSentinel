// REST client to cost-dashboard prescriptive API
// Maps MCP tool calls to cost-dashboard HTTP endpoints

use std::sync::LazyLock;

static DASHBOARD_URL: LazyLock<String> = LazyLock::new(|| {
    std::env::var("DASHBOARD_URL").unwrap_or_else(|_| "http://localhost:3001".into())
});

static DASHBOARD_API_KEY: LazyLock<String> = LazyLock::new(|| {
    std::env::var("DASHBOARD_API_KEY").unwrap_or_default()
});

async fn get(path: &str) -> Result<serde_json::Value, String> {
    let url = format!("{}{}", *DASHBOARD_URL, path);
    let client = reqwest::Client::new();
    let mut req = client.get(&url);
    if !DASHBOARD_API_KEY.is_empty() {
        req = req.header("X-Api-Key", &*DASHBOARD_API_KEY);
    }
    let resp = req.send().await.map_err(|e| format!("request failed: {e}"))?;
    resp.json::<serde_json::Value>().await.map_err(|e| format!("json decode: {e}"))
}

async fn post(path: &str, body: &serde_json::Value) -> Result<serde_json::Value, String> {
    let url = format!("{}{}", *DASHBOARD_URL, path);
    let client = reqwest::Client::new();
    let mut req = client.post(&url).json(body);
    if !DASHBOARD_API_KEY.is_empty() {
        req = req.header("X-Api-Key", &*DASHBOARD_API_KEY);
    }
    let resp = req.send().await.map_err(|e| format!("request failed: {e}"))?;
    resp.json::<serde_json::Value>().await.map_err(|e| format!("json decode: {e}"))
}

pub async fn get_cost_summary(period: &str) -> Result<serde_json::Value, String> {
    get(&format!("/api/dashboard/summary?period={period}")).await
}

pub async fn get_model_costs(period: &str) -> Result<serde_json::Value, String> {
    get(&format!("/api/dashboard/costs?period={period}")).await
}

pub async fn get_anomalies(period: &str) -> Result<serde_json::Value, String> {
    get(&format!("/api/dashboard/anomalies?period={period}")).await
}

pub async fn run_assessment(assessment_id: i64) -> Result<serde_json::Value, String> {
    post(&format!("/api/prescriptive/assessments/{assessment_id}/run"), &serde_json::json!({})).await
}

pub async fn run_whatif(assessment_id: i64, adjustments: &serde_json::Value) -> Result<serde_json::Value, String> {
    post(&format!("/api/prescriptive/assessments/{assessment_id}/whatif"), adjustments).await
}

pub async fn get_budget_status(team: &str) -> Result<serde_json::Value, String> {
    get(&format!("/api/admin/budgets/{team}")).await
}

pub async fn list_budget_rules() -> Result<serde_json::Value, String> {
    get("/api/admin/budget-rules").await
}

pub async fn get_report(assessment_id: i64) -> Result<serde_json::Value, String> {
    get(&format!("/api/prescriptive/assessments/{assessment_id}/report")).await
}
