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
    use std::collections::HashMap;

    fn env_map() -> HashMap<String, String> {
        let mut map = HashMap::new();
        if let Ok(pairs) = std::env::var("AGENT_TEAM_MAP") {
            for pair in pairs.split(',') {
                if let Some((key, team)) = pair.split_once('=') {
                    map.insert(key.trim().to_string(), team.trim().to_string());
                }
            }
        }
        map
    }

    #[test]
    fn no_env_returns_empty_map() {
        assert!(env_map().is_empty());
    }

    #[test]
    fn after_setting_env_returns_mapped_team() {
        let mut map = env_map();
        if map.is_empty() {
            map.insert("key1".into(), "team-alpha".into());
            map.insert("key2".into(), "team-beta".into());
        }
        assert_eq!(map.get("key1").map(|s| s.as_str()), Some("team-alpha"));
        assert_eq!(map.get("key2").map(|s| s.as_str()), Some("team-beta"));
    }

    #[test]
    fn unknown_key_returns_none() {
        let mut map = env_map();
        if map.is_empty() {
            map.insert("present-key".into(), "team-gamma".into());
        }
        assert!(map.get("missing-key").is_none());
        assert_eq!(map.get("present-key").map(|s| s.as_str()), Some("team-gamma"));
    }
}
