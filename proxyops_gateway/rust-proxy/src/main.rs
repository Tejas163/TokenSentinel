mod circuit_breaker;
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
use redis::aio::ConnectionManager;
use reqwest::Client;
use std::sync::Arc;
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
}

#[tokio::main]
async fn main() {
    tracing_subscriber::registry()
        .with(tracing_subscriber::fmt::layer())
        .init();

    let client = Client::new();
    let redis_url = std::env::var("REDIS_URL").unwrap_or_else(|_| "redis://127.0.0.1:6379".into());
    let redis_client = redis::Client::open(redis_url.as_str()).unwrap();
    let go_router_url = std::env::var("GO_ROUTER_URL").unwrap_or_else(|_| "http://127.0.0.1:8080".into());
    let redis_conn = Arc::new(Mutex::new(
        redis_client.get_connection_manager().await.unwrap(),
    ));

    let state = AppState {
        client: client.clone(),
        circuit_breaker: Arc::new(CircuitBreaker::new(5, Duration::from_secs(30), Duration::from_secs(5))),
        redis_conn: redis_conn.clone(),
        go_router_url: go_router_url.clone(),
    };

    let proxy_routes = Router::new()
        .route("/{*path}", get(handler).post(handler))
        .layer(axum::middleware::from_fn(request_id::request_id_middleware))
        .layer(TraceLayer::new_for_http());

    let app = Router::new()
        .route("/health", get(health_handler))
        .merge(proxy_routes)
        .with_state(state);

    let listener = tokio::net::TcpListener::bind("0.0.0.0:3000").await.unwrap();
    axum::serve(listener, app).await.unwrap();
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

async fn handler(
    State(state): State<AppState>,
    req: Request<Body>,
) -> Result<Response<Body>, axum::http::StatusCode> {
    let (parts, body) = req.into_parts();
    const MAX_BODY_SIZE: usize = 10 * 1024 * 1024;
    let body_bytes = axum::body::to_bytes(body, MAX_BODY_SIZE)
        .await
        .map_err(|_| axum::http::StatusCode::INTERNAL_SERVER_ERROR)?;
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

    state
        .circuit_breaker
        .call(|| async {
            let response = state
                .client
                .request(parts.method, target_url)
                .headers(filtered_headers)
                .body(body_bytes)
                .send()
                .await
                .map_err(|_| axum::http::StatusCode::BAD_GATEWAY)?;

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
        .await
}
