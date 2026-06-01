use std::sync::Mutex;
use std::time::{Duration, Instant};
use std::future::Future;

pub struct CircuitBreaker {
    threshold: usize,
    cooldown: Duration,
    state: Mutex<State>,
    timeout: Duration,
}

enum State {
    Closed { failures: u32 },
    Open { since: Instant },
    HalfOpen,
}

impl CircuitBreaker {
    pub fn new(threshold: usize, cooldown: Duration, timeout: Duration) -> Self {
        Self {
            threshold,
            cooldown,
            state: Mutex::new(State::Closed { failures: 0 }),
            timeout,
        }
    }

    pub async fn call<F, Fut, T, E>(&self, f: F) -> Result<T, E>
    where
        F: FnOnce() -> Fut,
        Fut: Future<Output = Result<T, E>>,
        E: From<CircuitOpen>,
    {
        {
            let mut state = self.state.lock().unwrap();
            match &*state {
                State::Open { since } if since.elapsed() < self.cooldown => {
                    return Err(CircuitOpen.into());
                }
                State::Open { .. } => {
                    *state = State::HalfOpen;
                }
                _ => {}
            }
        }

        let result=tokio::time::timeout(self.timeout, f()).await;
        let result = match result {
            Ok(inner) => inner,
            Err(_) => Err(CircuitOpen.into()),
        };
        self.record(result.is_ok());
        result
    }

    fn record(&self, success: bool) {
        let mut state = self.state.lock().unwrap();
        match (&mut *state, success) {
            (State::Closed { failures }, false) => {
                *failures += 1;
                if *failures >= self.threshold as u32 {
                    *state = State::Open { since: Instant::now() };
                }
            }
            (State::Closed { .. }, true) => {
                *state = State::Closed { failures: 0 };
            }
            (State::HalfOpen, false) => {
                *state = State::Open { since: Instant::now() };
            }
            (State::HalfOpen, true) => {
                *state = State::Closed { failures: 0 };
            }
            _ => {}
        }
    }
}

#[derive(Debug)]
pub struct CircuitOpen;


impl From<CircuitOpen> for axum::http::StatusCode {
    fn from(_: CircuitOpen) -> Self {
        axum::http::StatusCode::SERVICE_UNAVAILABLE
    }
}

impl std::fmt::Display for CircuitOpen {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "Service Unavailable")
    }
}

impl std::error::Error for CircuitOpen {}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::time::Duration;

    #[tokio::test]
    async fn closed_on_success_resets_failures() {
        let cb = Arc::new(CircuitBreaker::new(3, Duration::from_secs(30), Duration::from_secs(5)));
        let cb_ref = cb.clone();
        let result = cb_ref.call(|| async { Ok::<_, CircuitOpen>("ok") }).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), "ok");
        let state = cb.state.lock().unwrap();
        match &*state {
            State::Closed { failures } => assert_eq!(*failures, 0),
            _ => panic!("expected Closed state"),
        }
    }

    #[tokio::test]
    async fn opens_after_threshold_failures() {
        let cb = Arc::new(CircuitBreaker::new(3, Duration::from_secs(30), Duration::from_secs(5)));
        for _ in 0..3 {
            let cb_ref = cb.clone();
            let result = cb_ref
                .call(|| async { Err::<(), CircuitOpen>(CircuitOpen) })
                .await;
            assert!(result.is_err());
        }
        let state = cb.state.lock().unwrap();
        match &*state {
            State::Open { since: _ } => {}
            _ => panic!("expected Open state after threshold failures"),
        }
    }

    #[tokio::test]
    async fn rejects_requests_when_open() {
        let cb = Arc::new(CircuitBreaker::new(2, Duration::from_secs(30), Duration::from_secs(5)));
        for _ in 0..2 {
            let cb_ref = cb.clone();
            let _ = cb_ref.call(|| async { Err::<(), CircuitOpen>(CircuitOpen) }).await;
        }
        let cb_ref = cb.clone();
        let result = cb_ref.call(|| async { Ok::<_, CircuitOpen>("should not run") }).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn transitions_to_half_open_after_cooldown() {
        let cb = Arc::new(CircuitBreaker::new(2, Duration::from_millis(50), Duration::from_secs(5)));
        for _ in 0..2 {
            let cb_ref = cb.clone();
            let _ = cb_ref.call(|| async { Err::<(), CircuitOpen>(CircuitOpen) }).await;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
        {
            let state = cb.state.lock().unwrap();
            match &*state {
                State::Open { since } => {
                    assert!(since.elapsed() >= Duration::from_millis(50));
                }
                _ => panic!("expected Open before call"),
            }
        }
        let cb_ref = cb.clone();
        let result = cb_ref
            .call(|| async { Ok::<_, CircuitOpen>("recovered") })
            .await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), "recovered");
        let state = cb.state.lock().unwrap();
        match &*state {
            State::Closed { failures } => assert_eq!(*failures, 0),
            _ => panic!("expected Closed after successful half-open call"),
        }
    }

    #[tokio::test]
    async fn half_open_failure_reopens() {
        let cb = Arc::new(CircuitBreaker::new(2, Duration::from_millis(50), Duration::from_secs(5)));
        for _ in 0..2 {
            let cb_ref = cb.clone();
            let _ = cb_ref.call(|| async { Err::<(), CircuitOpen>(CircuitOpen) }).await;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
        let cb_ref = cb.clone();
        let result = cb_ref
            .call(|| async { Err::<(), CircuitOpen>(CircuitOpen) })
            .await;
        assert!(result.is_err());
        let state = cb.state.lock().unwrap();
        match &*state {
            State::Open { since: _ } => {}
            _ => panic!("expected Open after failed half-open call"),
        }
    }

    #[tokio::test]
    async fn respects_per_call_timeout() {
        let cb = Arc::new(CircuitBreaker::new(3, Duration::from_secs(30), Duration::from_millis(50)));
        let cb_ref = cb.clone();
        let result = cb_ref
            .call(|| async {
                tokio::time::sleep(Duration::from_secs(1)).await;
                Ok::<_, CircuitOpen>("too slow")
            })
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn closes_after_success() {
        let cb = Arc::new(CircuitBreaker::new(1, Duration::from_millis(50), Duration::from_secs(5)));
        let cb_ref = cb.clone();
        let _ = cb_ref.call(|| async { Err::<(), CircuitOpen>(CircuitOpen) }).await;
        tokio::time::sleep(Duration::from_millis(100)).await;
        let cb_ref = cb.clone();
        let _ = cb_ref.call(|| async { Ok::<_, CircuitOpen>("recovered") }).await;
        let state = cb.state.lock().unwrap();
        match &*state {
            State::Closed { failures } => assert_eq!(*failures, 0),
            _ => panic!("expected Closed after recovery"),
        }
    }

    #[tokio::test]
    async fn display_and_debug() {
        let err = CircuitOpen;
        assert_eq!(format!("{}", err), "Service Unavailable");
        assert!(!format!("{:?}", err).is_empty());
    }
}
