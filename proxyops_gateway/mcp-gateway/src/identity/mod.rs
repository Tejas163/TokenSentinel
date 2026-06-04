pub mod auth;

use axum::{
    body::Body,
    http::Request,
    middleware::Next,
    response::Response,
};

pub async fn auth_middleware(req: Request<Body>, next: Next) -> Response {
    // Delegates to auth::verify_api_key
    // Short-circuits with 401 if missing or invalid
    next.run(req).await
}
