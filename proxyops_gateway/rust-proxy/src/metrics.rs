use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::AtomicU64;

#[derive(Clone)]
pub struct Metrics {
    pub requests_total: Arc<AtomicU64>,
    pub requests_active: Arc<AtomicU64>,
    #[allow(dead_code)]
    pub requests_by_path: Arc<HashMap<String, Arc<AtomicU64>>>,
    pub latency_ms_sum: Arc<AtomicU64>,
    pub latency_ms_count: Arc<AtomicU64>,
    pub circuit_breaker_opens: Arc<AtomicU64>,
    pub upstream_errors: Arc<AtomicU64>,
}

impl Metrics {
    pub fn new() -> Self {
        let mut by_path = HashMap::new();
        for path in &["/health", "/metrics", "/mcp/", "/proxy"] {
            by_path.insert(path.to_string(), Arc::new(AtomicU64::new(0)));
        }
        Self {
            requests_total: Arc::new(AtomicU64::new(0)),
            requests_active: Arc::new(AtomicU64::new(0)),
            requests_by_path: Arc::new(by_path),
            latency_ms_sum: Arc::new(AtomicU64::new(0)),
            latency_ms_count: Arc::new(AtomicU64::new(0)),
            circuit_breaker_opens: Arc::new(AtomicU64::new(0)),
            upstream_errors: Arc::new(AtomicU64::new(0)),
        }
    }
}

impl Default for Metrics {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_metrics_new() {
        let m = Metrics::new();
        use std::sync::atomic::Ordering;
        assert_eq!(m.requests_total.load(Ordering::Relaxed), 0);
        assert_eq!(m.requests_active.load(Ordering::Relaxed), 0);
        assert_eq!(m.latency_ms_sum.load(Ordering::Relaxed), 0);
        assert_eq!(m.latency_ms_count.load(Ordering::Relaxed), 0);
        assert_eq!(m.circuit_breaker_opens.load(Ordering::Relaxed), 0);
        assert_eq!(m.upstream_errors.load(Ordering::Relaxed), 0);
    }

    #[test]
    fn test_metrics_increment() {
        let m = Metrics::new();
        use std::sync::atomic::Ordering;
        m.requests_total.fetch_add(5, Ordering::Relaxed);
        assert_eq!(m.requests_total.load(Ordering::Relaxed), 5);
        m.upstream_errors.fetch_add(2, Ordering::Relaxed);
        assert_eq!(m.upstream_errors.load(Ordering::Relaxed), 2);
        m.latency_ms_sum.fetch_add(150, Ordering::Relaxed);
        m.latency_ms_count.fetch_add(3, Ordering::Relaxed);
        assert_eq!(m.latency_ms_sum.load(Ordering::Relaxed), 150);
        assert_eq!(m.latency_ms_count.load(Ordering::Relaxed), 3);
    }

    #[test]
    fn test_metrics_clone_independent() {
        let m1 = Metrics::new();
        let m2 = m1.clone();
        use std::sync::atomic::Ordering;
        m1.requests_total.fetch_add(1, Ordering::Relaxed);
        assert_eq!(m2.requests_total.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_metrics_by_path_init() {
        let m = Metrics::new();
        assert!(m.requests_by_path.contains_key("/health"));
        assert!(m.requests_by_path.contains_key("/metrics"));
    }
}
