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

    let api_key = std::env::var("MCP_API_KEY").unwrap_or_default();
    let redis_url = std::env::var("REDIS_URL").unwrap_or_default();

    if api_key.is_empty() && redis_url.is_empty() {
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

    let used_key = if !header_key.is_empty() {
        header_key
    } else if !bearer_key.is_empty() {
        bearer_key
    } else {
        tracing::warn!("mcp auth failure: {}", req.uri().path());
        return Err(StatusCode::UNAUTHORIZED);
    };

    match auth::verify_api_key(used_key).await {
        Some((team, scopes)) => {
            let team = team.or_else(|| {
                let env_team = scoping::team_for_api_key(used_key);
                env_team
            });
            req.extensions_mut().insert(AgentInfo {
                agent_id: None,
                team,
                scopes,
            });
            Ok(next.run(req).await)
        }
        None => {
            tracing::warn!("mcp auth failure: {}", req.uri().path());
            Err(StatusCode::UNAUTHORIZED)
        }
    }
}
