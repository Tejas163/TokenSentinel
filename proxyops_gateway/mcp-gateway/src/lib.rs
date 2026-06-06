pub mod mcp;
pub mod identity;
pub mod prescriptive;
pub mod redis;

use axum::{Router, routing::{get, post}, middleware};
use mcp::transport::SessionStore;

pub fn create_app() -> Router {
    let session_store = SessionStore::new();

    let mcp_routes = Router::new()
        .route("/sse", get(mcp::transport::handle_sse))
        .route("/message", post(mcp::transport::handle_message))
        .layer(middleware::from_fn(identity::auth_middleware));

    let public_routes = Router::new()
        .route("/health", get(health_handler))
        .route("/metrics", get(metrics_handler));

    Router::new()
        .nest("/mcp/v1", mcp_routes)
        .merge(public_routes)
        .with_state(session_store)
}

async fn health_handler() -> &'static str {
    match redis::check_health().await {
        Ok(pong) => {
            tracing::debug!("health: redis {pong}");
            r#"{"status":"ok","redis":"connected"}"#
        }
        Err(e) => {
            tracing::warn!("health: redis degraded: {e}");
            r#"{"status":"degraded","redis":"disconnected"}"#
        }
    }
}

async fn metrics_handler() -> String {
    format!("# mcp-gateway metrics\n# health: see /health endpoint\n")
}
