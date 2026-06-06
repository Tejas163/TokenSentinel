mod circuit_breaker;
mod metrics;
mod request_id;

use axum::{
    body::Body,
    extract::State,
    http::{HeaderMap, HeaderName, Request, header},
    response::Response,
    routing::get,
    Router,
};
use circuit_breaker::CircuitBreaker;
use metrics::Metrics;
use redis::aio::ConnectionManager;
use reqwest::Client;
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tokio::sync::Mutex;
use tower_http::trace::TraceLayer;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[derive(Clone)]
struct AppState {
    client: Client,
    circuit_breaker: Arc<CircuitBreaker>,
    redis_conn: Arc<Mutex<ConnectionManager>>,
    go_router_url: String,
    mcp_gateway_url: String,
    metrics: Metrics,
}

#[tokio::main]
async fn main() {
    tracing_subscriber::registry()
        .with(tracing_subscriber::fmt::layer())
        .init();

    let client = Client::builder()
        .timeout(Duration::from_secs(30))
        .build()
        .unwrap();
    let redis_url = std::env::var("REDIS_URL").unwrap_or_else(|_| "redis://127.0.0.1:6379".into());
    let redis_client = redis::Client::open(redis_url.as_str()).unwrap();
    let go_router_url = std::env::var("GO_ROUTER_URL").unwrap_or_else(|_| "http://127.0.0.1:8080".into());
    let mcp_gateway_url = std::env::var("MCP_GATEWAY_URL").unwrap_or_else(|_| "http://127.0.0.1:3010".into());
    let redis_conn = Arc::new(Mutex::new(
        redis_client.get_connection_manager().await.unwrap(),
    ));
    let metrics = Metrics::new();

    let state = AppState {
        client: client.clone(),
        circuit_breaker: Arc::new(CircuitBreaker::new(5, Duration::from_secs(30), Duration::from_secs(5))),
        redis_conn: redis_conn.clone(),
        go_router_url: go_router_url.clone(),
        mcp_gateway_url: mcp_gateway_url.clone(),
        metrics: metrics.clone(),
    };

    let mcp_routes = Router::new()
        .route("/{*path}", get(mcp_handler).post(mcp_handler))
        .layer(axum::middleware::from_fn(request_id::request_id_middleware))
        .layer(TraceLayer::new_for_http());

    let proxy_routes = Router::new()
        .route("/{*path}", get(handler).post(handler))
        .layer(axum::middleware::from_fn(request_id::request_id_middleware))
        .layer(TraceLayer::new_for_http());

    let app = Router::new()
        .route("/health", get(health_handler))
        .route("/metrics", get(metrics_handler))
        .nest("/mcp", mcp_routes)
        .merge(proxy_routes)
        .with_state(state.clone())
        .layer(axum::middleware::from_fn_with_state(state, metrics_middleware));

    let listener = tokio::net::TcpListener::bind("0.0.0.0:3000").await.unwrap();
    tracing::info!("rust-proxy listening on :3000");
    axum::serve(listener, app).await.unwrap();
}

async fn metrics_middleware(
    State(state): State<AppState>,
    req: Request<Body>,
    next: axum::middleware::Next,
) -> Result<Response<Body>, axum::http::StatusCode> {
    let m = &state.metrics;
    let start = std::time::Instant::now();
    m.requests_active.fetch_add(1, Ordering::Relaxed);
    let result = next.run(req).await;
    m.requests_active.fetch_sub(1, Ordering::Relaxed);
    let latency = start.elapsed().as_millis() as u64;
    m.requests_total.fetch_add(1, Ordering::Relaxed);
    m.latency_ms_sum.fetch_add(latency, Ordering::Relaxed);
    m.latency_ms_count.fetch_add(1, Ordering::Relaxed);
    Ok(result)
}

async fn health_handler(
    State(state): State<AppState>,
) -> Result<Response<Body>, axum::http::StatusCode> {
    let mut conn = state.redis_conn.lock().await;
    redis::cmd("PING")
        .query_async::<()>(&mut *conn)
        .await
        .map(|_| Response::new(Body::from(r#"{"status":"ok"}"#)))
        .map_err(|_| axum::http::StatusCode::SERVICE_UNAVAILABLE)
}

async fn metrics_handler(
    State(state): State<AppState>,
) -> Result<Response<Body>, axum::http::StatusCode> {
    let m = &state.metrics;
    let total = m.requests_total.load(Ordering::Relaxed);
    let active = m.requests_active.load(Ordering::Relaxed);
    let latency_sum = m.latency_ms_sum.load(Ordering::Relaxed);
    let latency_count = m.latency_ms_count.load(Ordering::Relaxed);
    let cb_opens = m.circuit_breaker_opens.load(Ordering::Relaxed);
    let upstream_errors = m.upstream_errors.load(Ordering::Relaxed);

    let avg_latency = if latency_count > 0 {
        latency_sum / latency_count
    } else {
        0
    };

    let mut body = String::new();
    body.push_str("# HELP proxy_requests_total Total requests proxied\n");
    body.push_str("# TYPE proxy_requests_total counter\n");
    body.push_str(&format!("proxy_requests_total {} {}\n", total, chrono::Utc::now().timestamp()));

    body.push_str("# HELP proxy_requests_active Currently active requests\n");
    body.push_str("# TYPE proxy_requests_active gauge\n");
    body.push_str(&format!("proxy_requests_active {}\n", active));

    body.push_str("# HELP proxy_requests_latency_ms Average latency in milliseconds\n");
    body.push_str("# TYPE proxy_requests_latency_ms gauge\n");
    body.push_str(&format!("proxy_requests_latency_ms {}\n", avg_latency));

    body.push_str("# HELP proxy_circuit_breaker_opens_total Circuit breaker opens\n");
    body.push_str("# TYPE proxy_circuit_breaker_opens_total counter\n");
    body.push_str(&format!("proxy_circuit_breaker_opens_total {}\n", cb_opens));

    body.push_str("# HELP proxy_upstream_errors_total Upstream (Go Router) errors\n");
    body.push_str("# TYPE proxy_upstream_errors_total counter\n");
    body.push_str(&format!("proxy_upstream_errors_total {}\n", upstream_errors));

    if let Some(redis_health) = check_redis_health(&state).await {
        body.push_str("# HELP proxy_redis_health Redis health (1=healthy, 0=unhealthy)\n");
        body.push_str("# TYPE proxy_redis_health gauge\n");
        body.push_str(&format!("proxy_redis_health {}\n", redis_health as u64));
    }

    body.push_str("# HELP proxy_build_info Build metadata\n");
    body.push_str("# TYPE proxy_build_info gauge\n");
    body.push_str("proxy_build_info{version=\"0.1.0\",rust=\"stable\"} 1\n");

    Ok(Response::new(Body::from(body)))
}

async fn check_redis_health(state: &AppState) -> Option<bool> {
    let mut conn = state.redis_conn.lock().await;
    redis::cmd("PING")
        .query_async::<String>(&mut *conn)
        .await
        .ok()
        .map(|r| r == "PONG")
}

async fn mcp_handler(
    State(state): State<AppState>,
    req: Request<Body>,
) -> Result<Response<Body>, axum::http::StatusCode> {
    let (parts, body) = req.into_parts();
    let body_bytes = axum::body::to_bytes(body, 10 * 1024 * 1024)
        .await
        .map_err(|e| {
            tracing::error!("mcp body read error: {e}");
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        })?;
    let target_url = format!("{}{}", state.mcp_gateway_url, parts.uri.path());

    let mut filtered_headers = parts.headers.clone();
    filtered_headers.remove("host");

    let response = state
        .client
        .request(parts.method, &target_url)
        .headers(filtered_headers)
        .body(body_bytes)
        .send()
        .await
        .map_err(|e| {
            tracing::error!("mcp upstream error: {e}");
            state.metrics.upstream_errors.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            axum::http::StatusCode::BAD_GATEWAY
        })?;

    let status = response.status();
    let headers = response.headers().clone();
    let body = response
        .bytes()
        .await
        .map_err(|_| axum::http::StatusCode::BAD_GATEWAY)?;

    let mut builder = Response::builder().status(status);
    for (name, value) in headers.iter() {
        builder = builder.header(name, value);
    }
    Ok(builder.body(Body::from(body)).unwrap())
}

async fn handler(
    State(state): State<AppState>,
    req: Request<Body>,
) -> Result<Response<Body>, axum::http::StatusCode> {
    let (parts, body) = req.into_parts();
    const MAX_BODY_SIZE: usize = 10 * 1024 * 1024;
    let body_bytes = axum::body::to_bytes(body, MAX_BODY_SIZE)
        .await
        .map_err(|e| {
            tracing::error!("body read error: {e}");
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        })?;
    let target_url = format!("{}{}", state.go_router_url, parts.uri.path());

    let safe_request_headers: &[HeaderName] = &[
        header::CONTENT_TYPE,
        header::CONTENT_LENGTH,
        header::ACCEPT,
        header::ACCEPT_ENCODING,
        header::HOST,
        header::USER_AGENT,
        HeaderName::from_static("x-request-id"),
        HeaderName::from_static("x-forwarded-for"),
    ];
    let mut filtered_headers = HeaderMap::new();
    for h in safe_request_headers {
        if let Some(v) = parts.headers.get(h) {
            filtered_headers.insert(h.clone(), v.clone());
        }
    }

    let metrics = state.metrics.clone();
    let client = state.client.clone();
    let cb = &state.circuit_breaker;
    let cb_result = cb
        .call(|| async {
            let response = client
                .request(parts.method, target_url)
                .headers(filtered_headers)
                .body(body_bytes)
                .send()
                .await
                .map_err(|e| {
                    tracing::error!("upstream error: {e}");
                    metrics.upstream_errors.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
                    axum::http::StatusCode::BAD_GATEWAY
                })?;

            let status = response.status();
            let headers = response.headers().clone();
            let body = response
                .bytes()
                .await
                .map_err(|_| axum::http::StatusCode::BAD_GATEWAY)?;

            let safe_response_headers: &[HeaderName] = &[
                header::CONTENT_TYPE,
                header::CONTENT_LENGTH,
                header::CACHE_CONTROL,
                header::LOCATION,
                HeaderName::from_static("x-request-id"),
            ];
            let mut builder = Response::builder().status(status);
            for (name, value) in headers {
                if let Some(n) = name {
                    if safe_response_headers.contains(&n) {
                        builder = builder.header(n, value);
                    }
                }
            }
            Ok(builder.body(Body::from(body)).unwrap())
        })
        .await;

    if cb_result.is_err() {
        state.metrics.circuit_breaker_opens.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
    }
    cb_result
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{body::Body, http::{Request, StatusCode}, routing::get};
    use tower::ServiceExt;

    /// Helper: creates an AppState for handler tests. Requires a running Redis on :26379.
    /// Tests using this are #[ignore] by default; run with `cargo test -- --ignored` with Redis.
    async fn live_state(metrics: Metrics) -> AppState {
        AppState {
            client: Client::new(),
            circuit_breaker: Arc::new(CircuitBreaker::new(3, Duration::from_secs(30), Duration::from_secs(5))),
            redis_conn: Arc::new(Mutex::new(
                redis::Client::open("redis://127.0.0.1:26379").unwrap()
                    .get_connection_manager().await.unwrap()
            )),
            go_router_url: "http://127.0.0.1:18080".into(),
            mcp_gateway_url: "http://127.0.0.1:13010".into(),
            metrics,
        }
    }

    #[tokio::test]
    #[ignore]
    async fn health_without_redis_returns_503() {
        let app = Router::new()
            .route("/health", get(health_handler))
            .with_state(live_state(Metrics::new()).await);

        let resp = app
            .oneshot(Request::builder().uri("/health").body(Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::SERVICE_UNAVAILABLE);
    }

    #[tokio::test]
    #[ignore]
    async fn metrics_handler_output() {
        let m = Metrics::new();
        m.requests_total.fetch_add(42, Ordering::Relaxed);
        m.latency_ms_sum.fetch_add(1000, Ordering::Relaxed);
        m.latency_ms_count.fetch_add(10, Ordering::Relaxed);

        let app = Router::new()
            .route("/metrics", get(metrics_handler))
            .with_state(live_state(m).await);

        let resp = app
            .oneshot(Request::builder().uri("/metrics").body(Body::empty()).unwrap())
            .await
            .unwrap();

        assert_eq!(resp.status(), StatusCode::OK);
        let body = String::from_utf8(
            axum::body::to_bytes(resp.into_body(), usize::MAX)
                .await
                .unwrap()
                .to_vec(),
        )
        .unwrap();

        assert!(body.contains("proxy_requests_total 42"));
        assert!(body.contains("proxy_build_info"));
    }
}
