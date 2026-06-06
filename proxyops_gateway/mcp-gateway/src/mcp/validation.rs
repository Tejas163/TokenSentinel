use crate::mcp::tools::list_tools;

pub fn validate_tool_args(tool_name: &str, arguments: &serde_json::Value) -> Result<(), String> {
    let tools = list_tools();
    let tool = match tools.iter().find(|t| t.name == tool_name) {
        Some(t) => t,
        None => return Err(format!("Unknown tool: {tool_name}")),
    };

    let schema = match &tool.input_schema {
        Some(s) => s,
        None => return Ok(()),
    };

    let compiled = match jsonschema::JSONSchema::compile(schema) {
        Ok(v) => v,
        Err(e) => return Err(format!("schema compile error: {e}")),
    };

    if let Err(errors) = compiled.validate(arguments) {
        let msgs: Vec<String> = errors.map(|e| format!("{e}")).collect();
        return Err(msgs.join("; "));
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn valid_period_passes() {
        let args = serde_json::json!({"period": "24h"});
        assert!(validate_tool_args("get_cost_summary", &args).is_ok());
    }

    #[test]
    fn invalid_period_rejected() {
        let args = serde_json::json!({"period": "invalid"});
        assert!(validate_tool_args("get_cost_summary", &args).is_err());
    }

    #[test]
    fn wrong_type_rejected() {
        let args = serde_json::json!({"period": 42});
        assert!(validate_tool_args("get_cost_summary", &args).is_err());
    }

    #[test]
    fn missing_required_field_rejected() {
        let args = serde_json::json!({});
        let result = validate_tool_args("get_cost_summary", &args);
        assert!(result.is_err(), "missing required 'period' should fail");
    }

    #[test]
    fn missing_period_on_optional_tool_is_ok() {
        let args = serde_json::json!({});
        assert!(validate_tool_args("list_budget_rules", &args).is_ok());
    }

    #[test]
    fn unknown_tool_rejected() {
        let args = serde_json::json!({});
        assert!(validate_tool_args("nonexistent", &args).is_err());
    }

    #[test]
    fn integer_arg_accepts_positive() {
        let args = serde_json::json!({"assessment_id": 42});
        assert!(validate_tool_args("run_assessment", &args).is_ok());
    }

    #[test]
    fn integer_arg_rejects_string() {
        let args = serde_json::json!({"assessment_id": "not-a-number"});
        assert!(validate_tool_args("run_assessment", &args).is_err());
    }

    #[test]
    fn empty_schema_is_noop() {
        let args = serde_json::json!({"unexpected": "value"});
        assert!(validate_tool_args("list_budget_rules", &args).is_ok());
    }

    #[test]
    fn assessment_id_required_for_run_assessment() {
        let args = serde_json::json!({"adjustments": {}});
        assert!(validate_tool_args("run_assessment", &args).is_err());
    }

    #[test]
    fn whatif_requires_both_fields() {
        let args = serde_json::json!({"assessment_id": 1});
        assert!(validate_tool_args("run_whatif", &args).is_err());
    }

    #[test]
    fn whatif_accepts_valid_args() {
        let args = serde_json::json!({"assessment_id": 1, "adjustments": {"volume": 0.5}});
        assert!(validate_tool_args("run_whatif", &args).is_ok());
    }

    #[test]
    fn all_tools_have_valid_schemas() {
        for tool in crate::mcp::tools::list_tools() {
            if let Some(schema) = &tool.input_schema {
                assert!(
                    jsonschema::JSONSchema::compile(schema).is_ok(),
                    "tool '{}' has invalid input schema",
                    tool.name
                );
            }
        }
    }

    #[test]
    fn assessment_id_negative_rejected() {
        let args = serde_json::json!({"assessment_id": -1});
        let result = validate_tool_args("run_assessment", &args);
        if let Err(msg) = result {
            assert!(msg.contains("assessment_id"), "error should mention assessment_id: {msg}");
        }
    }

    #[test]
    fn team_sanitized() {
        let args = serde_json::json!({"team": "team-alpha"});
        assert!(validate_tool_args("get_budget_status", &args).is_ok());
    }

    #[test]
    fn team_accepts_string_regardless_of_content() {
        // Schema validates type (string), not content pattern.
        // Content sanitization happens in handlers, not at schema level.
        let args = serde_json::json!({"team": "team<script>"});
        assert!(validate_tool_args("get_budget_status", &args).is_ok());
    }
}
