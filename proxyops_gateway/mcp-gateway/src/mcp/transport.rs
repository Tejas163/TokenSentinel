use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use axum::extract::{Query, State, Extension};
use axum::response::sse::{Event, Sse};
use axum::response::IntoResponse;
use axum::Json;
use crate::identity::AgentInfo;
use tokio_stream::Stream;
use serde::{Deserialize, Serialize};
use tokio::sync::{mpsc, Mutex};
use tokio_stream::wrappers::ReceiverStream;
use tokio_stream::StreamExt;
use std::convert::Infallible;

#[derive(Debug, Deserialize)]
pub struct JsonRpcRequest {
    pub jsonrpc: String,
    pub id: Option<serde_json::Value>,
    pub method: String,
    pub params: Option<serde_json::Value>,
}

#[derive(Debug, Serialize)]
pub struct JsonRpcResponse {
    pub jsonrpc: String,
    pub id: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<JsonRpcError>,
}

#[derive(Debug, Serialize)]
pub struct JsonRpcError {
    pub code: i32,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<serde_json::Value>,
}

impl JsonRpcResponse {
    pub fn success(id: Option<serde_json::Value>, result: serde_json::Value) -> Self {
        Self { jsonrpc: "2.0".into(), id, result: Some(result), error: None }
    }

    pub fn error(id: Option<serde_json::Value>, code: i32, message: &str) -> Self {
        Self {
            jsonrpc: "2.0".into(), id,
            result: None,
            error: Some(JsonRpcError { code, message: message.into(), data: None }),
        }
    }

    pub fn internal_error(id: Option<serde_json::Value>, message: &str) -> Self {
        Self::error(id, -32603, message)
    }
}

#[derive(Clone)]
pub struct SessionStore {
    sessions: Arc<Mutex<HashMap<String, mpsc::Sender<Event>>>>,
}

impl SessionStore {
    pub fn new() -> Self {
        Self { sessions: Arc::new(Mutex::new(HashMap::new())) }
    }

    pub async fn register(&self, id: String, tx: mpsc::Sender<Event>) {
        self.sessions.lock().await.insert(id, tx);
    }

    pub async fn send(&self, session_id: &str, event: Event) -> Result<(), String> {
        let sessions = self.sessions.lock().await;
        if let Some(tx) = sessions.get(session_id) {
            tx.send(event).await.map_err(|e| format!("session send: {e}"))
        } else {
            Err(format!("session {session_id} not found"))
        }
    }

    pub async fn remove(&self, id: &str) {
        self.sessions.lock().await.remove(id);
    }
}

#[derive(Deserialize)]
pub struct SessionQuery {
    pub session_id: Option<String>,
}

pub async fn handle_sse(
    State(store): State<SessionStore>,
) -> Sse<impl Stream<Item = Result<Event, Infallible>>> {
    let session_id = uuid::Uuid::new_v4().to_string();
    let (tx, rx) = mpsc::channel::<Event>(64);

    store.register(session_id.clone(), tx).await;

    let store_clone = store.clone();
    let sid = session_id.clone();
    tokio::spawn(async move {
        tokio::time::sleep(Duration::from_secs(60)).await;
        store_clone.remove(&sid).await;
    });

    let endpoint_event = Event::default()
        .event("endpoint")
        .data(format!("/mcp/v1/message?session_id={session_id}"));

    let stream = tokio_stream::once(Ok(endpoint_event))
        .chain(ReceiverStream::new(rx).map(Ok));

    Sse::new(stream).keep_alive(
        axum::response::sse::KeepAlive::new()
            .interval(Duration::from_secs(15))
            .text("keep-alive"),
    )
}

pub async fn handle_message(
    State(store): State<SessionStore>,
    Query(query): Query<SessionQuery>,
    Extension(agent): Extension<AgentInfo>,
    Json(req): Json<JsonRpcRequest>,
) -> impl IntoResponse {
    let session_id = match &query.session_id {
        Some(id) => id.clone(),
        None => return Json(JsonRpcResponse::error(None, -32000, "session_id required")),
    };

    let response = crate::mcp::dispatch(req, agent).await;

    let event = Event::default()
        .event("message")
        .data(serde_json::to_string(&response).unwrap_or_default());

    match store.send(&session_id, event).await {
        Ok(()) => Json(JsonRpcResponse::success(response.id.clone(), serde_json::json!({"accepted": true}))),
        Err(e) => Json(JsonRpcResponse::internal_error(response.id, &e)),
    }
}
