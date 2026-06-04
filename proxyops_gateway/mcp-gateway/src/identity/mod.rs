pub mod auth;

use axum::{
    http::Request,
    middleware::Next,
    response::Response,
};

pub async fn auth_middleware<B>(req: Request<B>, next: Next<B>) -> Response {
    // Delegates to auth::verify_api_key
    // Short-circuits with 401 if missing or invalid
    next.run(req).await
}
