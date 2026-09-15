use anyhow::{anyhow, Result};
use reqwest::{Client, StatusCode};
use serde::{Deserialize, Serialize};
use tracing::{error, info};

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

#[derive(Debug, Clone, Deserialize)]
pub struct GatewayState {
    pub machines: usize,
    pub active_machines: usize,
    pub client_connections: i64,
    pub sessions: i64,
    pub machine_states: Vec<MachineState>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct MachineState {
    pub id: String,
    pub address: String,
    pub state: i32,
    pub sessions: i32,
    pub last_heartbeat: i64,
}

impl ApiClient {
    pub fn new(base_url: &str) -> Self {
        Self {
            client: Client::new(),
            base_url: base_url.to_string(),
        }
    }

    pub fn get_base_url(&self) -> &str {
        &self.base_url
    }

    pub async fn get_metrics(&self) -> Result<String> {
        let url = format!("{}/metrics", self.base_url);
        let resp = self.client.get(url).send().await?;
        if resp.status() != StatusCode::OK {
            return Err(anyhow!("Metrics API HTTP error: {}", resp.status()));
        }
        Ok(resp.text().await?)
    }

    pub async fn get_gateway_state(&self) -> Result<GatewayState> {
        let url = format!("{}/state", self.base_url);
        let resp = self.client.get(url).send().await?;
        if resp.status() != StatusCode::OK {
            return Err(anyhow!("Gateway state HTTP error: {}", resp.status()));
        }
        let api_resp: ApiResponse<GatewayState> = resp.json().await?;
        if api_resp.code != 200 && api_resp.code != 0 {
            return Err(anyhow!(
                "Gateway state business error: {}",
                api_resp.message
            ));
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
            return Err(anyhow!(
                "API Business Error ({}): {}",
                api_resp.code,
                api_resp.message
            ));
        }
        Ok(api_resp.data)
    }

    pub async fn create_ticket(
        &self,
        workflow_name: &str,
        properties: serde_json::Value,
    ) -> Result<String> {
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
            return Err(anyhow!(
                "API Business Error ({}): {}",
                api_resp.code,
                api_resp.message
            ));
        }

        info!(
            "API: Session acquired: {}***",
            &api_resp.data.session_id[..6]
        );
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
