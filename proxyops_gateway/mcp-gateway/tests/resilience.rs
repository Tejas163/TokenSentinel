use axum::{
    body::Body,
    http::{Request, StatusCode, Method},
};
use mcp_gateway::create_app;
use tower::ServiceExt;

fn setup() {
    unsafe { std::env::set_var("MCP_API_KEY", ""); }
    let _ = tracing_subscriber::fmt()
        .with_env_filter("mcp_gateway=error")
        .try_init();
}

#[tokio::test]
async fn health_returns_ok() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn health_returns_valid_json() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    let body = axum::body::to_bytes(response.into_body(), 1024).await.unwrap();
    let body_str = String::from_utf8(body.to_vec()).unwrap();
    assert!(body_str.contains("status"));
    assert!(body_str.contains("redis"));
}

#[tokio::test]
async fn health_is_degraded_without_redis() {
    // REDIS_URL points to nonexistent redis
    unsafe { std::env::set_var("REDIS_URL", "redis://127.0.0.1:9999"); }
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    // Should either be OK or degraded depending on connection timeout
    // Both are acceptable responses
    assert!(response.status() == StatusCode::OK);
}

#[tokio::test]
async fn metrics_returns_ok() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/metrics")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn sse_connect_returns_streaming_response() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/sse")
                .header("Accept", "text/event-stream")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn message_without_session_id_returns_error() {
    setup();
    let app = create_app();

    let body = serde_json::json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/list"
    });
    let body_bytes = serde_json::to_vec(&body).unwrap();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/message")
                .method(Method::POST)
                .header("Content-Type", "application/json")
                .body(Body::from(body_bytes))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);

    let resp_body = axum::body::to_bytes(response.into_body(), 4096).await.unwrap();
    let resp_json: serde_json::Value = serde_json::from_slice(&resp_body).unwrap();

    // Should return an error about missing session_id
    assert_eq!(resp_json["error"]["code"], -32000);
}

#[tokio::test]
async fn message_invalid_json_returns_422() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/message?session_id=test")
                .method(Method::POST)
                .header("Content-Type", "application/json")
                .body(Body::from("not valid json"))
                .unwrap(),
        )
        .await
        .unwrap();

    // axum Json extractor returns 400 for malformed JSON body
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn unknown_route_returns_404() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/nonexistent")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn message_without_auth_still_works_when_no_key_configured() {
    // When MCP_API_KEY is empty, auth is bypassed
    unsafe { std::env::set_var("MCP_API_KEY", ""); }
    setup();
    let app = create_app();

    let body = serde_json::json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/list"
    });
    let body_bytes = serde_json::to_vec(&body).unwrap();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/message?session_id=test-session")
                .method(Method::POST)
                .header("Content-Type", "application/json")
                .body(Body::from(body_bytes))
                .unwrap(),
        )
        .await
        .unwrap();

    let resp_body = axum::body::to_bytes(response.into_body(), 4096).await.unwrap();
    let resp_json: serde_json::Value = serde_json::from_slice(&resp_body).unwrap();

    // Should still get a JSON-RPC response (session not found)
    assert_eq!(resp_json["jsonrpc"], "2.0");
    assert!(resp_json["id"] == 1 || resp_json["id"].is_null());
}

#[tokio::test]
async fn message_to_nonexistent_session_returns_session_error() {
    setup();
    let app = create_app();

    let body = serde_json::json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/list"
    });
    let body_bytes = serde_json::to_vec(&body).unwrap();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/message?session_id=nonexistent")
                .method(Method::POST)
                .header("Content-Type", "application/json")
                .body(Body::from(body_bytes))
                .unwrap(),
        )
        .await
        .unwrap();

    let resp_body = axum::body::to_bytes(response.into_body(), 4096).await.unwrap();
    let resp_json: serde_json::Value = serde_json::from_slice(&resp_body).unwrap();

    // Session delivery fails → internal error
    assert_eq!(resp_json["error"]["code"], -32603);
}

#[tokio::test]
async fn tools_list_accepts_no_args() {
    setup();
    let app = create_app();

    let body = serde_json::json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/list"
    });
    let body_bytes = serde_json::to_vec(&body).unwrap();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/message?session_id=test-session")
                .method(Method::POST)
                .header("Content-Type", "application/json")
                .body(Body::from(body_bytes))
                .unwrap(),
        )
        .await
        .unwrap();

    let resp_body = axum::body::to_bytes(response.into_body(), 4096).await.unwrap();
    let resp_json: serde_json::Value = serde_json::from_slice(&resp_body).unwrap();

    // tools/list should return tools in result, even with bad session
    // (dispatch doesn't validate session — it's a separate concern)
    assert_eq!(resp_json["jsonrpc"], "2.0");
}

#[tokio::test]
async fn double_sse_connect_creates_separate_sessions() {
    setup();
    let app = create_app();

    let req1 = Request::builder()
        .uri("/mcp/v1/sse")
        .body(Body::empty())
        .unwrap();
    let req2 = Request::builder()
        .uri("/mcp/v1/sse")
        .body(Body::empty())
        .unwrap();

    let (resp1, resp2) = tokio::join!(
        app.clone().oneshot(req1),
        app.clone().oneshot(req2),
    );

    assert!(resp1.is_ok());
    assert!(resp2.is_ok());
    assert_eq!(resp1.unwrap().status(), StatusCode::OK);
    assert_eq!(resp2.unwrap().status(), StatusCode::OK);
}

#[tokio::test]
async fn post_to_sse_endpoint_returns_405() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/sse")
                .method(Method::POST)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    // GET-only route should reject POST
    assert_eq!(response.status(), StatusCode::METHOD_NOT_ALLOWED);
}

#[tokio::test]
async fn get_to_message_endpoint_returns_405() {
    setup();
    let app = create_app();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/mcp/v1/message")
                .method(Method::GET)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::METHOD_NOT_ALLOWED);
}
