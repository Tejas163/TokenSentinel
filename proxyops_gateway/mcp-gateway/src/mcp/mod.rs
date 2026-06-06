pub mod transport;
pub mod tools;
pub mod handlers;
pub mod handlers_whatif;
pub mod validation;
pub mod budget;

use transport::{JsonRpcRequest, JsonRpcResponse};
use tools::list_tools;
use crate::identity::AgentInfo;

fn truncate_args(args: &serde_json::Value, max_len: usize) -> String {
    let s = serde_json::to_string(args).unwrap_or_default();
    if s.len() > max_len {
        format!("{}... (truncated)", &s[..max_len])
    } else {
        s
    }
}

pub async fn dispatch(req: JsonRpcRequest, agent: AgentInfo) -> JsonRpcResponse {
    let request_id = uuid::Uuid::new_v4().to_string();
    let span = tracing::info_span!("dispatch", request_id = %request_id, method = %req.method);
    let _enter = span.enter();

    let resp = match req.method.as_str() {
        "tools/list" => handle_tools_list(req),
        "tools/call" => handle_tools_call(req, agent, &request_id).await,
        _ => JsonRpcResponse::error(req.id, -32601, "Method not found"),
    };

    if resp.error.is_some() {
        tracing::warn!(
            request_id = %request_id,
            method = %req.method,
            error_code = resp.error.as_ref().map(|e| e.code),
            error_message = resp.error.as_ref().map(|e| e.message.as_str()),
            "rpc error"
        );
    }

    resp
}

fn handle_tools_list(req: JsonRpcRequest) -> JsonRpcResponse {
    let tools = list_tools();
    JsonRpcResponse::success(req.id, serde_json::json!({ "tools": tools }))
}

async fn handle_tools_call(req: JsonRpcRequest, agent: AgentInfo, request_id: &str) -> JsonRpcResponse {
    let tool_name = match req.params.as_ref()
        .and_then(|p| p.get("name"))
        .and_then(|n| n.as_str())
    {
        Some(name) => name.to_string(),
        None => return JsonRpcResponse::error(req.id, -32602, "Missing required parameter: name"),
    };

    let arguments = req.params.as_ref()
        .and_then(|p| p.get("arguments"))
        .cloned()
        .unwrap_or(serde_json::Value::Object(serde_json::Map::new()));

    let args_truncated = truncate_args(&arguments, 200);

    tracing::info!(
        request_id = %request_id,
        tool = %tool_name,
        team = ?agent.team,
        args = %args_truncated,
        "tool call"
    );

    if !agent.scopes.contains(&"*".to_string())
        && !agent.scopes.iter().any(|s| s == &format!("tools:{tool_name}"))
    {
        return JsonRpcResponse::error(
            req.id,
            -32000,
            &format!("Scope 'tools:{tool_name}' not granted for agent '{}'", agent.agent_id.as_deref().unwrap_or("unknown")),
        );
    }

    if let Err(e) = budget::check_tool_allowed(&tool_name, agent.team.as_deref()).await {
        return JsonRpcResponse::error(req.id, -32000, &e);
    }

    if let Err(e) = validation::validate_tool_args(&tool_name, &arguments) {
        return JsonRpcResponse::error(req.id, -32602, &e);
    }

    let result = match tool_name.as_str() {
        "get_cost_summary" => {
            let period = arguments.get("period").and_then(|v| v.as_str()).unwrap_or("24h");
            handlers::handle_get_cost_summary(period, agent.team.as_deref()).await
        }
        "get_model_costs" => {
            let period = arguments.get("period").and_then(|v| v.as_str()).unwrap_or("24h");
            handlers::handle_get_model_costs(period, agent.team.as_deref()).await
        }
        "get_anomalies" => {
            let period = arguments.get("period").and_then(|v| v.as_str()).unwrap_or("24h");
            handlers::handle_get_anomalies(period, agent.team.as_deref()).await
        }
        "run_assessment" => {
            let id = arguments.get("assessment_id").and_then(|v| v.as_i64()).unwrap_or(0);
            handlers::handle_run_assessment(id, agent.team.as_deref()).await
        }
        "run_whatif" => {
            let id = arguments.get("assessment_id").and_then(|v| v.as_i64()).unwrap_or(0);
            let adj = arguments.get("adjustments").cloned().unwrap_or(serde_json::Value::Null);
            handlers::handle_run_whatif(id, adj, agent.team.as_deref()).await
        }
        "whatif_multi_scenario" => {
            let id = arguments.get("assessment_id").and_then(|v| v.as_i64()).unwrap_or(0);
            let scenarios = arguments.get("scenarios").and_then(|v| v.as_array()).cloned().unwrap_or_default();
            handlers_whatif::handle_whatif_multi_scenario(id, scenarios, agent.team.as_deref()).await
        }
        "whatif_volume_shift" => {
            let id = arguments.get("assessment_id").and_then(|v| v.as_i64()).unwrap_or(0);
            let pct = arguments.get("volume_pct").and_then(|v| v.as_f64()).unwrap_or(0.0);
            handlers_whatif::handle_whatif_volume_shift(id, pct, agent.team.as_deref()).await
        }
        "whatif_model_switch" => {
            let id = arguments.get("assessment_id").and_then(|v| v.as_i64()).unwrap_or(0);
            let from = arguments.get("from_model").and_then(|v| v.as_str()).unwrap_or("");
            let to = arguments.get("to_model").and_then(|v| v.as_str()).unwrap_or("");
            handlers_whatif::handle_whatif_model_switch(id, from, to, agent.team.as_deref()).await
        }
        "get_budget_status" => {
            let request_team = arguments.get("team").and_then(|v| v.as_str())
                .or(agent.team.as_deref())
                .unwrap_or("");
            handlers::handle_get_budget_status(request_team).await
        }
        "list_budget_rules" => {
            handlers::handle_list_budget_rules(agent.team.as_deref()).await
        }
        "get_report" => {
            let id = arguments.get("assessment_id").and_then(|v| v.as_i64()).unwrap_or(0);
            handlers::handle_get_report(id, agent.team.as_deref()).await
        }
        _ => return JsonRpcResponse::error(req.id, -32601, &format!("Unknown tool: {tool_name}")),
    };

    match result {
        Ok(value) => {
            tracing::info!(
                request_id = %request_id,
                tool = %tool_name,
                result_size = serde_json::to_string(&value).map(|s| s.len()).unwrap_or(0),
                "tool success"
            );
            JsonRpcResponse::success(req.id, serde_json::json!({ "content": [
                { "type": "text", "text": serde_json::to_string(&value).unwrap_or_default() }
            ]}))
        }
        Err(e) => {
            tracing::warn!(
                request_id = %request_id,
                tool = %tool_name,
                error = %e,
                "tool error"
            );
            JsonRpcResponse::internal_error(req.id, &e)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_agent() -> AgentInfo {
        AgentInfo {
            agent_id: None,
            team: None,
            scopes: vec!["*".into()],
        }
    }

    #[tokio::test]
    async fn unknown_method_returns_32601() {
        let req = JsonRpcRequest {
            jsonrpc: "2.0".into(),
            id: Some(serde_json::json!(1)),
            method: "nonexistent".into(),
            params: None,
        };
        let resp = dispatch(req, test_agent()).await;
        assert_eq!(resp.error.as_ref().unwrap().code, -32601);
    }

    #[tokio::test]
    async fn tools_list_returns_ok() {
        let req = JsonRpcRequest {
            jsonrpc: "2.0".into(),
            id: Some(serde_json::json!(1)),
            method: "tools/list".into(),
            params: None,
        };
        let resp = dispatch(req, test_agent()).await;
        assert!(resp.result.is_some());
        assert!(resp.error.is_none());
    }

    #[tokio::test]
    async fn scope_granted_allows_tool() {
        let req = JsonRpcRequest {
            jsonrpc: "2.0".into(),
            id: Some(serde_json::json!(1)),
            method: "tools/call".into(),
            params: Some(serde_json::json!({
                "name": "list_budget_rules",
                "arguments": {}
            })),
        };
        let agent = AgentInfo {
            agent_id: Some("test-agent".into()),
            team: None,
            scopes: vec!["tools:list_budget_rules".into(), "tools:get_cost_summary".into()],
        };
        let resp = dispatch(req, agent).await;
        // Scope is granted — should not return scope error
        if let Some(ref err) = resp.error {
            assert_ne!(err.code, -32000, "scope was denied: {}", err.message);
        }
    }

    #[tokio::test]
    async fn scope_denied_blocks_tool() {
        let req = JsonRpcRequest {
            jsonrpc: "2.0".into(),
            id: Some(serde_json::json!(1)),
            method: "tools/call".into(),
            params: Some(serde_json::json!({
                "name": "run_assessment",
                "arguments": {"assessment_id": 1}
            })),
        };
        let agent = AgentInfo {
            agent_id: Some("restricted-agent".into()),
            team: None,
            scopes: vec!["tools:get_cost_summary".into()],
        };
        let resp = dispatch(req, agent).await;
        assert_eq!(resp.error.as_ref().unwrap().code, -32000);
        assert!(resp.error.as_ref().unwrap().message.contains("run_assessment"));
    }

    #[tokio::test]
    async fn wildcard_scope_allows_any_tool() {
        let req = JsonRpcRequest {
            jsonrpc: "2.0".into(),
            id: Some(serde_json::json!(1)),
            method: "tools/call".into(),
            params: Some(serde_json::json!({
                "name": "run_assessment",
                "arguments": {"assessment_id": 1}
            })),
        };
        let agent = AgentInfo {
            agent_id: None,
            team: None,
            scopes: vec!["*".into()],
        };
        let resp = dispatch(req, agent).await;
        // Should try to execute (will fail because no upstream, but not scope error)
        if let Some(ref err) = resp.error {
            assert_ne!(err.code, -32000, "wildcard scope was denied: {}", err.message);
        }
    }
}
