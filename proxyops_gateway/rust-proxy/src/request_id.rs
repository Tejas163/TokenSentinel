use axum::{
    body::Body,
    extract::Request,
    middleware::Next,
    response::Response,
    http::StatusCode,
};

use uuid::Uuid;

#[derive(Clone,Debug)]
#[allow(dead_code)]
pub struct RequestId(pub String);

pub async fn request_id_middleware(mut req: Request<Body>, next: Next) -> Result<Response<Body>, StatusCode> {
    let id = req
        .headers()
        .get("X-Request-ID")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
        .unwrap_or_else(|| Uuid::new_v4().to_string());

    req.extensions_mut().insert(RequestId(id.clone()));
    req.headers_mut().insert("X-Request-ID", id.parse().unwrap());
    Ok(next.run(req).await)
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{Router, routing::get, body::Body, http::Request};
    use tower::ServiceExt;

    async fn ext_handler(req: Request<Body>) -> String {
        req.extensions()
            .get::<RequestId>()
            .map(|id| id.0.clone())
            .unwrap_or_else(|| "none".to_string())
    }

    async fn header_handler(req: Request<Body>) -> String {
        req.headers()
            .get("X-Request-ID")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("none")
            .to_string()
    }

    fn make_app() -> Router {
        Router::new()
            .route("/ext", get(ext_handler))
            .route("/header", get(header_handler))
            .layer(axum::middleware::from_fn(request_id_middleware))
    }

    #[tokio::test]
    async fn injects_request_id_into_extensions_when_missing() {
        let app = make_app();
        let resp = app
            .oneshot(Request::builder().uri("/ext").body(Body::empty()).unwrap())
            .await
            .unwrap();
        let body = String::from_utf8(
            axum::body::to_bytes(resp.into_body(), usize::MAX)
                .await
                .unwrap()
                .to_vec(),
        )
        .unwrap();
        assert_eq!(body.len(), 36, "expected UUID v4 string");
        assert_eq!(body.chars().filter(|&c| c == '-').count(), 4);
    }

    #[tokio::test]
    async fn injects_request_id_into_headers_when_missing() {
        let app = make_app();
        let resp = app
            .oneshot(Request::builder().uri("/header").body(Body::empty()).unwrap())
            .await
            .unwrap();
        let body = String::from_utf8(
            axum::body::to_bytes(resp.into_body(), usize::MAX)
                .await
                .unwrap()
                .to_vec(),
        )
        .unwrap();
        assert_eq!(body.len(), 36, "expected UUID v4 string");
        assert_eq!(body.chars().filter(|&c| c == '-').count(), 4);
    }

    #[tokio::test]
    async fn preserves_existing_request_id_in_extensions() {
        let app = make_app();
        let resp = app
            .oneshot(
                Request::builder()
                    .uri("/ext")
                    .header("X-Request-ID", "my-trace-1")
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        let body = String::from_utf8(
            axum::body::to_bytes(resp.into_body(), usize::MAX)
                .await
                .unwrap()
                .to_vec(),
        )
        .unwrap();
        assert_eq!(body, "my-trace-1");
    }

    #[tokio::test]
    async fn preserves_existing_request_id_in_headers() {
        let app = make_app();
        let resp = app
            .oneshot(
                Request::builder()
                    .uri("/header")
                    .header("X-Request-ID", "my-trace-2")
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        let body = String::from_utf8(
            axum::body::to_bytes(resp.into_body(), usize::MAX)
                .await
                .unwrap()
                .to_vec(),
        )
        .unwrap();
        assert_eq!(body, "my-trace-2");
    }
}
