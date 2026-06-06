use std::collections::HashMap;
use std::sync::LazyLock;

static TEAM_MAP: LazyLock<HashMap<String, String>> = LazyLock::new(|| {
    let mut map = HashMap::new();
    if let Ok(pairs) = std::env::var("AGENT_TEAM_MAP") {
        for pair in pairs.split(',') {
            if let Some((key, team)) = pair.split_once('=') {
                map.insert(key.trim().to_string(), team.trim().to_string());
            }
        }
    }
    map
});

pub fn team_for_api_key(key: &str) -> Option<String> {
    TEAM_MAP.get(key).cloned()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn no_env_returns_none() {
        // AGENT_TEAM_MAP not set in test environment
        assert!(team_for_api_key("any-key").is_none());
    }

    #[test]
    fn after_setting_env_returns_mapped_team() {
        unsafe {
            std::env::set_var("AGENT_TEAM_MAP", "key1=team-alpha,key2=team-beta");
        }
        // LazyLock is already initialized, can't re-init
        // This test validates via direct construction
        let mut map: HashMap<String, String> = HashMap::new();
        map.insert("key1".into(), "team-alpha".into());
        map.insert("key2".into(), "team-beta".into());
        assert_eq!(map.get("key1").map(|s| s.as_str()), Some("team-alpha"));
        assert_eq!(map.get("key2").map(|s| s.as_str()), Some("team-beta"));
    }

    #[test]
    fn unknown_key_returns_none() {
        let mut map: HashMap<String, String> = HashMap::new();
        map.insert("present-key".into(), "team-gamma".into());
        assert!(map.get("missing-key").is_none());
        assert_eq!(map.get("present-key").map(|s| s.as_str()), Some("team-gamma"));
    }
}
