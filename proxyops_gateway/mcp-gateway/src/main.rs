#![allow(dead_code)]

mod mcp;
mod a2a;
mod identity;
mod prescriptive;
mod redis;

use axum::{Router, routing::{get, post}};

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter("mcp_gateway=debug,tower_http=debug")
        .init();

    let app = Router::new()
        .route("/mcp/v1", post(mcp::handle_mcp_request))
        .route("/a2a/v1", post(a2a::handle_a2a_request))
        .route("/health", get(health))
        .route("/metrics", get(metrics));

    let listener = tokio::net::TcpListener::bind("0.0.0.0:3010")
        .await
        .expect("bind mcp-gateway on :3010");

    tracing::info!("mcp-gateway listening on :3010 (MCP), :3011 (A2A)");

    axum::serve(listener, app).await.unwrap();
}

async fn health() -> &'static str {
    r#"{"status":"ok"}"#
}

async fn metrics() -> &'static str {
    "# mcp-gateway metrics placeholder\n"
}
