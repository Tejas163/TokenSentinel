use std::sync::LazyLock;
use std::time::Duration;
use reqwest::Client;

static DASHBOARD_URL: LazyLock<String> = LazyLock::new(|| {
    std::env::var("DASHBOARD_URL").unwrap_or_else(|_| "http://localhost:3001".into())
});

static DASHBOARD_API_KEY: LazyLock<String> = LazyLock::new(|| {
    std::env::var("DASHBOARD_API_KEY").unwrap_or_default()
});

fn http_client() -> Client {
    Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .expect("reqwest client")
}

async fn get(path: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    let url = format!("{}{}", *DASHBOARD_URL, path);
    let client = http_client();
    let mut last_err = String::new();
    for attempt in 0..3 {
        if attempt > 0 {
            tokio::time::sleep(Duration::from_millis(100 * 2u64.pow(attempt as u32))).await;
        }
        let mut req = client.get(&url);
        if !DASHBOARD_API_KEY.is_empty() {
            req = req.header("X-Api-Key", &*DASHBOARD_API_KEY);
        }
        if let Some(t) = team {
            req = req.header("X-Team-Name", t);
        }
        match req.send().await {
            Ok(resp) => {
                if resp.status().is_success() {
                    return resp.json::<serde_json::Value>().await.map_err(|e| format!("json decode: {e}"));
                }
                last_err = format!("HTTP {}", resp.status());
            }
            Err(e) => {
                last_err = format!("request failed: {e}");
                if !e.is_connect() && !e.is_timeout() {
                    break;
                }
            }
        }
    }
    Err(last_err)
}

async fn post(path: &str, body: &serde_json::Value, team: Option<&str>) -> Result<serde_json::Value, String> {
    let url = format!("{}{}", *DASHBOARD_URL, path);
    let client = http_client();
    let mut last_err = String::new();
    for attempt in 0..3 {
        if attempt > 0 {
            tokio::time::sleep(Duration::from_millis(100 * 2u64.pow(attempt as u32))).await;
        }
        let mut req = client.post(&url).json(body);
        if !DASHBOARD_API_KEY.is_empty() {
            req = req.header("X-Api-Key", &*DASHBOARD_API_KEY);
        }
        if let Some(t) = team {
            req = req.header("X-Team-Name", t);
        }
        match req.send().await {
            Ok(resp) => {
                if resp.status().is_success() {
                    return resp.json::<serde_json::Value>().await.map_err(|e| format!("json decode: {e}"));
                }
                last_err = format!("HTTP {}", resp.status());
            }
            Err(e) => {
                last_err = format!("request failed: {e}");
                if !e.is_connect() && !e.is_timeout() {
                    break;
                }
            }
        }
    }
    Err(last_err)
}

pub async fn get_cost_summary(period: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    get(&format!("/api/dashboard/summary?period={period}"), team).await
}

pub async fn get_model_costs(period: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    get(&format!("/api/dashboard/costs?period={period}"), team).await
}

pub async fn get_anomalies(period: &str, team: Option<&str>) -> Result<serde_json::Value, String> {
    get(&format!("/api/dashboard/anomalies?period={period}"), team).await
}

pub async fn run_assessment(assessment_id: i64, team: Option<&str>) -> Result<serde_json::Value, String> {
    post(&format!("/api/prescriptive/assessments/{assessment_id}/run"), &serde_json::json!({}), team).await
}

pub async fn run_whatif(assessment_id: i64, adjustments: &serde_json::Value, team: Option<&str>) -> Result<serde_json::Value, String> {
    post(&format!("/api/prescriptive/assessments/{assessment_id}/whatif"), adjustments, team).await
}

pub async fn get_budget_status(team: &str) -> Result<serde_json::Value, String> {
    get(&format!("/api/budget/status?team={team}"), None).await
}

pub async fn list_budget_rules() -> Result<serde_json::Value, String> {
    get("/api/admin/budget-rules", None).await
}

pub async fn list_budget_rules_with_team(team: Option<&str>) -> Result<serde_json::Value, String> {
    get("/api/admin/budget-rules", team).await
}

pub async fn get_report(assessment_id: i64, team: Option<&str>) -> Result<serde_json::Value, String> {
    get(&format!("/api/prescriptive/report/{assessment_id}"), team).await
}

pub async fn get_model_catalog() -> Result<serde_json::Value, String> {
    get("/api/prescriptive/models", None).await
}
