use serde::{Deserialize, Serialize};
use std::collections::HashMap;

use crate::redis;

#[derive(Debug, Serialize, Deserialize)]
pub struct AgentClaims {
    pub sub: String,
    pub team: Option<String>,
    pub exp: u64,
    pub iat: u64,
    pub scopes: Vec<String>,
}

pub async fn verify_api_key(key: &str) -> Option<(Option<String>, Vec<String>)> {
    if let Ok(mut conn) = redis::get_connection().await {
        let fields: HashMap<String, String> = ::redis::cmd("HGETALL")
            .arg(format!("apikey:{}", key))
            .query_async(&mut conn)
            .await
            .unwrap_or_default();

        if !fields.is_empty() {
            let status = fields.get("status").map(|s| s.as_str()).unwrap_or("active");
            if status != "active" {
                return None;
            }
            let team = fields.get("team").cloned().filter(|t| !t.is_empty());
            return Some((team, vec!["*".into()]));
        }
    }

    let valid_keys = std::env::var("MCP_API_KEY").unwrap_or_default();
    if valid_keys.split(',').any(|k| k.trim() == key) {
        return Some((None, vec!["*".into()]));
    }

    None
}

pub fn jwt_secret() -> Option<Vec<u8>> {
    std::env::var("JWT_SECRET").ok().map(|s| s.into_bytes())
}

pub fn issue_token(
    agent_id: &str,
    team: Option<&str>,
    secret: &[u8],
    scopes: &[String],
) -> Result<String, jsonwebtoken::errors::Error> {
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();

    let claims = AgentClaims {
        sub: agent_id.to_string(),
        team: team.map(String::from),
        exp: now + 3600,
        iat: now,
        scopes: scopes.to_vec(),
    };

    jsonwebtoken::encode(
        &jsonwebtoken::Header::default(),
        &claims,
        &jsonwebtoken::EncodingKey::from_secret(secret),
    )
}

pub fn verify_token(
    token: &str,
    secret: &[u8],
) -> Result<AgentClaims, jsonwebtoken::errors::Error> {
    let token_data = jsonwebtoken::decode::<AgentClaims>(
        token,
        &jsonwebtoken::DecodingKey::from_secret(secret),
        &jsonwebtoken::Validation::default(),
    )?;
    Ok(token_data.claims)
}
