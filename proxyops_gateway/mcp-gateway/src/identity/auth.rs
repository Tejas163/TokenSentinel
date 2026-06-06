use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct AgentClaims {
    pub sub: String,
    pub team: Option<String>,
    pub exp: u64,
    pub iat: u64,
    pub scopes: Vec<String>,
}

pub fn verify_api_key(key: &str) -> bool {
    let valid_keys = std::env::var("MCP_API_KEY").unwrap_or_default();
    valid_keys.split(',').any(|k| k.trim() == key)
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
