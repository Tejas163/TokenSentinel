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
