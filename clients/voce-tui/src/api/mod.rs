use anyhow::{Result, anyhow};
use reqwest::{Client, StatusCode};
use serde::{Deserialize, Serialize};
use tracing::{info, error};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct WorkflowConfig {
    pub id: String,
    pub name: String,
    pub version: String,
    pub head: String,
    pub nodes: Vec<NodeConfig>,
    pub edges: Vec<EdgeConfig>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct NodeConfig {
    pub id: String,
    pub name: String,
    pub plugin: String,
    pub config: serde_json::Value,
    #[serde(default)]
    pub metadata: std::collections::HashMap<String, serde_json::Value>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct EdgeConfig {
    pub source: String,
    pub source_port: i32,
    pub target: String,
    pub r#type: i32,
}

#[derive(Clone)]
pub struct ApiClient {
    client: Client,
    base_url: String,
}

#[derive(Deserialize)]
struct ApiResponse<T> {
    pub code: i32,
    pub message: String,
    pub data: T,
}

#[derive(Deserialize)]
struct SessionData {
    pub session_id: String,
}

#[derive(Debug, Serialize, Deserialize, Clone, Default)]
pub struct MonitorStats {
    pub goroutines: i32,
    pub heap_alloc: u64,
    pub heap_idle: u64,
    pub heap_inuse: u64,
    pub stack_inuse: u64,
    pub num_gc: u32,
    pub pause_total_ns: u64,
    pub last_gc_time: String,
    pub system_mem: u64,
    pub active_audio_count: i64,
    pub active_sd_video_count: i64,
    pub active_hd_video_count: i64,
    pub active_fhd_video_count: i64,
    pub active_sessions: i64,
    pub active_connections: i64,
    pub audio_traffic_in: u64,
    pub audio_traffic_out: u64,
    pub timestamp: i64,
}

impl ApiClient {
    pub fn new(base_url: &str) -> Self {
        Self { client: Client::new(), base_url: base_url.to_string() }
    }

    pub fn get_base_url(&self) -> &str {
        &self.base_url
    }

    pub async fn get_monitor_stats(&self) -> Result<MonitorStats> {
        let url = format!("{}/monitor", self.base_url);
        let resp = self.client.get(url).send().await?;
        
        if resp.status() != StatusCode::OK {
            return Err(anyhow!("Monitor API HTTP error: {}", resp.status()));
        }
        
        // Use the common ApiResponse wrapper
        let api_resp: ApiResponse<MonitorStats> = resp.json().await?;
        if api_resp.code != 200 && api_resp.code != 0 {
             return Err(anyhow!("Monitor API Business Error: {}", api_resp.message));
        }
        Ok(api_resp.data)
    }

    pub async fn list_workflows(&self) -> Result<Vec<WorkflowConfig>> {
        let url = format!("{}/workflows", self.base_url);
        info!("API: Fetching workflows from {}", url);
        let resp = self.client.get(url).send().await?;
        
        if resp.status() != StatusCode::OK {
            return Err(anyhow!("List Workflows HTTP error: {}", resp.status()));
        }
        
        let api_resp: ApiResponse<Vec<WorkflowConfig>> = resp.json().await?;
        if api_resp.code != 200 && api_resp.code != 0 {
             return Err(anyhow!("API Business Error ({}): {}", api_resp.code, api_resp.message));
        }
        Ok(api_resp.data)
    }

    pub async fn create_ticket(&self, workflow_name: &str, properties: serde_json::Value) -> Result<String> {
        let url = format!("{}/sessions", self.base_url);
        info!("API: Requesting session for workflow: {}", workflow_name);
        
        // Backend expects "name" (workflow name)
        let body = serde_json::json!({
            "name": workflow_name,
            "properties": properties
        });
        
        let resp = self.client.post(url).json(&body).send().await?;
        let status = resp.status();
        
        if status != StatusCode::OK {
            let detail = resp.text().await.unwrap_or_default();
            error!("API: Session request failed ({}): {}", status, detail);
            return Err(anyhow!("Create Session Error: {} - {}", status, detail));
        }

        let api_resp: ApiResponse<SessionData> = resp.json().await?;
        if api_resp.code != 200 && api_resp.code != 0 {
             return Err(anyhow!("API Business Error ({}): {}", api_resp.code, api_resp.message));
        }
        
        info!("API: Session acquired: {}***", &api_resp.data.session_id[..6]);
        Ok(api_resp.data.session_id)
    }

    pub async fn renew_session(&self, session_id: &str) -> Result<()> {
        let url = format!("{}/sessions/renew/{}", self.base_url, session_id);
        let resp = self.client.post(url).send().await?;
        
        if !resp.status().is_success() {
            return Err(anyhow!("Renew Session HTTP error: {}", resp.status()));
        }
        
        Ok(())
    }
}
