use axum::{
    body::Body,
    extract::State,
    http::{HeaderName, HeaderValue, Request, StatusCode},
    middleware::Next,
    response::Response,
};
use redis::AsyncCommands;
use std::collections::HashMap;

use crate::AppState;

pub async fn auth_middleware(
    State(state): State<AppState>,
    mut req: Request<Body>,
    next: Next,
) -> Result<Response<Body>, StatusCode> {
    let found_key = {
        let header_key = req
            .headers()
            .get("X-Api-Key")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("");

        let bearer_key = req
            .headers()
            .get("Authorization")
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.strip_prefix("Bearer "))
            .unwrap_or("");

        if !header_key.is_empty() {
            header_key.to_owned()
        } else if !bearer_key.is_empty() {
            bearer_key.to_owned()
        } else {
            tracing::warn!("auth: missing api key");
            return Err(StatusCode::UNAUTHORIZED);
        }
    };

    let mut conn = state.redis_conn.lock().await;

    let fields: HashMap<String, String> = conn
        .hgetall(format!("apikey:{}", found_key))
        .await
        .map_err(|e| {
            tracing::error!("auth: redis error: {e}");
            StatusCode::INTERNAL_SERVER_ERROR
        })?;

    if fields.is_empty() {
        tracing::warn!("auth: unknown key");
        return Err(StatusCode::UNAUTHORIZED);
    }

    let status = fields.get("status").map(|s| s.as_str()).unwrap_or("active");
    if status != "active" {
        tracing::warn!("auth: key inactive (status={})", status);
        return Err(StatusCode::UNAUTHORIZED);
    }

    req.headers_mut().insert(
        HeaderName::from_static("x-api-key"),
        HeaderValue::from_str(&found_key).unwrap(),
    );

    if let Some(team) = fields.get("team") {
        if !team.is_empty() {
            req.headers_mut().insert(
                HeaderName::from_static("x-team-name"),
                HeaderValue::from_str(team).unwrap(),
            );
        }
    }

    Ok(next.run(req).await)
}
