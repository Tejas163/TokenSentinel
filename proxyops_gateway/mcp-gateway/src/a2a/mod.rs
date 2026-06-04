pub mod register;
pub mod delegate;

use axum::response::IntoResponse;

pub async fn handle_a2a_request() -> impl IntoResponse {
    // A2A protocol dispatcher — delegates to register/delegate based on action
    axum::response::Json(serde_json::json!({
        "status": "not_implemented"
    }))
}
