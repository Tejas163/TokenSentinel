pub mod auth;
pub mod scoping;

use axum::{
    extract::Request,
    middleware::Next,
    response::Response,
    http::StatusCode,
};

#[derive(Clone, Debug)]
pub struct AgentInfo {
    pub agent_id: Option<String>,
    pub team: Option<String>,
    pub scopes: Vec<String>,
}

pub async fn auth_middleware(mut req: Request, next: Next) -> Result<Response, StatusCode> {
    // Try JWT first
    if let Some(secret) = auth::jwt_secret() {
        let token = req
            .headers()
            .get("Authorization")
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.strip_prefix("Bearer "))
            .unwrap_or("");

        if token.is_empty() {
            tracing::warn!("mcp auth failure: missing Bearer token (JWT mode)");
            return Err(StatusCode::UNAUTHORIZED);
        }

        match auth::verify_token(token, &secret) {
            Ok(claims) => {
                req.extensions_mut().insert(AgentInfo {
                    agent_id: Some(claims.sub),
                    team: claims.team,
                    scopes: claims.scopes,
                });
                return Ok(next.run(req).await);
            }
            Err(e) => {
                tracing::warn!("mcp auth failure: JWT invalid: {e}");
                return Err(StatusCode::UNAUTHORIZED);
            }
        }
    }

    // Fallback: API key auth (Phase 1 compatibility)
    let api_key = std::env::var("MCP_API_KEY").unwrap_or_default();
    if api_key.is_empty() {
        req.extensions_mut().insert(AgentInfo {
            agent_id: None,
            team: None,
            scopes: vec!["*".into()],
        });
        return Ok(next.run(req).await);
    }

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

    let used_key = if auth::verify_api_key(header_key) {
        header_key
    } else if auth::verify_api_key(bearer_key) {
        bearer_key
    } else {
        tracing::warn!("mcp auth failure: {}", req.uri().path());
        return Err(StatusCode::UNAUTHORIZED);
    };

    let team = scoping::team_for_api_key(used_key).await;
    req.extensions_mut().insert(AgentInfo {
        agent_id: None,
        team,
        scopes: vec!["*".into()],
    });
    Ok(next.run(req).await)
}
