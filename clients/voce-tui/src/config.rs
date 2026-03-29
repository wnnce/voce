use serde::{Deserialize, Serialize};
use std::fs;
use tracing::{info, warn};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct Config {
    #[serde(default = "default_api_url")]
    pub api_url: String,
}

fn default_api_url() -> String {
    "http://127.0.0.1:7001".to_string()
}

impl Config {
    pub fn load() -> Self {
        let config_path = "voce.toml";
        
        // Try read; if fails or unmarshal fails, create default
        if let Ok(content) = fs::read_to_string(config_path) {
            if let Ok(cfg) = toml::from_str(&content) {
                info!("Loaded config from {}", config_path);
                return cfg;
            }
            warn!("Failed to parse {}, using defaults", config_path);
        }

        let default_cfg = Self::default();
        let _ = Self::save_default(&default_cfg, config_path);
        default_cfg
    }

    fn save_default(cfg: &Self, path: &str) -> Result<(), std::io::Error> {
        if let Ok(toml_str) = toml::to_string_pretty(cfg) {
            fs::write(path, toml_str)?;
            info!("Created default config file at {}", path);
        }
        Ok(())
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            api_url: default_api_url(),
        }
    }
}
