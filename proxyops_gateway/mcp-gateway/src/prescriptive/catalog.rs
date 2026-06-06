use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;
use serde::{Deserialize, Serialize};
use crate::prescriptive::client;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModelInfo {
    pub name: String,
    pub provider: String,
    pub capability: String,
    pub input_price_per_1k: f64,
    pub output_price_per_1k: f64,
}

#[derive(Debug, Clone)]
pub struct CatalogState {
    models: Arc<RwLock<Vec<ModelInfo>>>,
    last_refresh: Arc<RwLock<Instant>>,
    refresh_interval: Duration,
}

impl CatalogState {
    pub fn new() -> Self {
        Self {
            models: Arc::new(RwLock::new(Vec::new())),
            last_refresh: Arc::new(RwLock::new(Instant::now())),
            refresh_interval: Duration::from_secs(300),
        }
    }

    pub async fn refresh(&self) {
        match client::get_model_catalog().await {
            Ok(raw) => {
                if let Ok(models) = serde_json::from_value::<Vec<ModelInfo>>(raw) {
                    *self.models.write().await = models;
                    *self.last_refresh.write().await = Instant::now();
                    tracing::info!("model catalog refreshed");
                } else {
                    tracing::warn!("model catalog parse failed");
                }
            }
            Err(e) => tracing::warn!("model catalog fetch failed: {e}"),
        }
    }

    pub async fn get_models(&self) -> Vec<ModelInfo> {
        if self.last_refresh.read().await.elapsed() > self.refresh_interval {
            tokio::spawn({
                let catalog = self.clone();
                async move { catalog.refresh().await }
            });
        }
        self.models.read().await.clone()
    }

    pub async fn cheapest_for_capability(&self, capability: &str) -> Option<ModelInfo> {
        let models = self.get_models().await;
        models
            .into_iter()
            .filter(|m| m.capability == capability)
            .min_by(|a, b| {
                a.input_price_per_1k
                    .partial_cmp(&b.input_price_per_1k)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
    }
}

pub fn start_catalog_refresh(catalog: CatalogState) {
    tokio::spawn(async move {
        catalog.refresh().await;
        loop {
            tokio::time::sleep(Duration::from_secs(300)).await;
            catalog.refresh().await;
        }
    });
}
