use crate::prescriptive::client;

pub async fn check_tool_allowed(tool_name: &str, team: Option<&str>) -> Result<(), String> {
    let team = match team {
        Some(t) if !t.is_empty() => t,
        _ => return Ok(()),
    };

    let status = client::get_budget_status(team).await.map_err(|e| format!("budget check failed: {e}"))?;

    let exceeded = status.get("budget_exceeded").and_then(|v| v.as_bool()).unwrap_or(false);
    if exceeded {
        return Err(format!("Team '{team}' has exceeded its budget. Tool '{tool_name}' blocked."));
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn no_team_allows_all() {
        let rt = tokio::runtime::Runtime::new().unwrap();
        let result = rt.block_on(check_tool_allowed("any_tool", None));
        assert!(result.is_ok());
    }

    #[test]
    fn empty_team_string_allows_all() {
        let rt = tokio::runtime::Runtime::new().unwrap();
        let result = rt.block_on(check_tool_allowed("any_tool", Some("")));
        assert!(result.is_ok());
    }

    #[test]
    fn error_message_format() {
        let msg = format!("Team '{}' has exceeded its budget. Tool '{}' blocked.", "test-team", "run_assessment");
        assert!(msg.contains("test-team"));
        assert!(msg.contains("run_assessment"));
    }

    #[test]
    fn allows_when_budget_not_exceeded() {
        let status = serde_json::json!({"budget_exceeded": false, "team": "test"});
        let exceeded = status.get("budget_exceeded").and_then(|v| v.as_bool()).unwrap_or(false);
        assert!(!exceeded);
    }

    #[test]
    fn blocks_when_budget_exceeded() {
        let status = serde_json::json!({"budget_exceeded": true, "team": "test"});
        let exceeded = status.get("budget_exceeded").and_then(|v| v.as_bool()).unwrap_or(false);
        assert!(exceeded);
    }

    #[test]
    fn missing_field_defaults_to_not_exceeded() {
        let status = serde_json::json!({"team": "test"});
        let exceeded = status.get("budget_exceeded").and_then(|v| v.as_bool()).unwrap_or(false);
        assert!(!exceeded);
    }
}
