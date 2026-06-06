use mcp_gateway::create_app;
use mcp_gateway::prescriptive::catalog::{CatalogState, start_catalog_refresh};

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter("mcp_gateway=debug,tower_http=debug")
        .init();

    let catalog = CatalogState::new();
    start_catalog_refresh(catalog);

    let app = create_app();

    let listener = tokio::net::TcpListener::bind("0.0.0.0:3010")
        .await
        .expect("bind mcp-gateway on :3010");

    tracing::info!("mcp-gateway listening on :3010");

    axum::serve(listener, app).await.unwrap();
}
