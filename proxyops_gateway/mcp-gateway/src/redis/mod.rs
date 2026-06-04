// Redis integration — shares existing TokenSentinel Redis instance
// Reads rate limits, health state, and route configuration
// Publishes cost events for dashboard consumption

use std::sync::LazyLock;
use redis::aio::ConnectionManager;

static REDIS_URL: LazyLock<String> = LazyLock::new(|| {
    std::env::var("REDIS_URL").unwrap_or_else(|_| "redis://127.0.0.1:6379".into())
});

pub async fn get_connection() -> Result<ConnectionManager, redis::RedisError> {
    let client = redis::Client::open(REDIS_URL.as_str())?;
    ConnectionManager::new(client).await
}

pub async fn check_health() -> Result<String, String> {
    let mut conn = get_connection().await.map_err(|e| format!("redis connect: {e}"))?;
    let pong: String = redis::cmd("PING")
        .query_async(&mut conn)
        .await
        .map_err(|e| format!("redis ping: {e}"))?;
    Ok(pong)
}
