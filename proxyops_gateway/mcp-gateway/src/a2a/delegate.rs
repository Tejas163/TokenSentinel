// Task delegation — route subtasks to registered agents and aggregate results
// Supports synchronous (wait for result) and asynchronous (callback/webhook) patterns

use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct TaskDelegation {
    pub task_id: String,
    pub target_agent: String,
    pub payload: serde_json::Value,
    pub response_mode: Option<String>, // "sync" | "async"
    pub webhook_url: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct TaskResult {
    pub task_id: String,
    pub status: String,
    pub result: Option<serde_json::Value>,
    pub error: Option<String>,
}
