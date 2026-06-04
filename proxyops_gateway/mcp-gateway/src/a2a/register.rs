// Agent registration — agents announce capabilities and identity
// Registered agents are stored in Redis for discovery and routing

use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct AgentRegistration {
    pub agent_id: String,
    pub capabilities: Vec<String>,
    pub endpoint: String,
    pub metadata: Option<serde_json::Value>,
}

#[derive(Debug, Serialize)]
pub struct RegistrationResponse {
    pub agent_id: String,
    pub status: String,
    pub issued_at: String,
}
