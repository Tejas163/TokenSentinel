pub mod transport;
pub mod tools;
pub mod handlers;

use axum::response::IntoResponse;

pub async fn handle_mcp_request() -> impl IntoResponse {
    // JSON-RPC dispatch for MCP protocol
    // Delegates to transport::dispatch
    axum::response::Json(serde_json::json!({
        "jsonrpc": "2.0",
        "id": null,
        "error": { "code": -32000, "message": "not implemented" }
    }))
}
