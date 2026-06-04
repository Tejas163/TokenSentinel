// Agent identity and API key authentication
// Supports static API keys (Phase 1) and JWK-signed identity tokens (Phase 2)

use jsonwebtoken::{decode, encode, DecodingKey, EncodingKey, Header, Validation};
use serde::{Deserialize, Serialize};
use std::collections::HashSet;
use std::sync::LazyLock;

static VALID_API_KEYS: LazyLock<HashSet<String>> = LazyLock::new(|| {
    let key = std::env::var("MCP_API_KEY").unwrap_or_default();
    let mut set = HashSet::new();
    if !key.is_empty() {
        set.insert(key);
    }
    set
});

#[derive(Debug, Serialize, Deserialize)]
pub struct AgentClaims {
    pub sub: String,       // agent_id
    pub exp: u64,          // expiry timestamp
    pub iat: u64,          // issued at
    pub scopes: Vec<String>, // "mcp:read", "mcp:write", "a2a:delegate"
}

pub fn verify_api_key(key: &str) -> bool {
    VALID_API_KEYS.contains(key)
}

pub fn issue_token(agent_id: &str, secret: &[u8]) -> Result<String, jsonwebtoken::errors::Error> {
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();

    let claims = AgentClaims {
        sub: agent_id.to_string(),
        exp: now + 3600, // 1 hour expiry
        iat: now,
        scopes: vec!["mcp:read".into(), "mcp:write".into(), "a2a:delegate".into()],
    };

    encode(&Header::default(), &claims, &EncodingKey::from_secret(secret))
}

pub fn verify_token(token: &str, secret: &[u8]) -> Result<AgentClaims, jsonwebtoken::errors::Error> {
    let token_data = decode::<AgentClaims>(
        token,
        &DecodingKey::from_secret(secret),
        &Validation::default(),
    )?;
    Ok(token_data.claims)
}
