use std::collections::VecDeque;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::time::Duration;
use once_cell::sync::Lazy;
use crate::api::{ApiClient, MonitorStats};

#[derive(Debug, Clone, Default)]
pub struct MonitorPoint {
    pub traffic_in_bps: u64,
    pub traffic_out_bps: u64,
}

pub struct MonitorData {
    pub history: VecDeque<MonitorPoint>,
    pub last_raw: Option<MonitorStats>,
}

/// The Global Repository for telemetry data
pub static MONITOR_DATA: Lazy<Arc<RwLock<MonitorData>>> = Lazy::new(|| {
    Arc::new(RwLock::new(MonitorData {
        history: VecDeque::with_capacity(120),
        last_raw: None,
    }))
});

pub struct MonitorWorker {
    api_client: Arc<ApiClient>,
}

impl MonitorWorker {
    pub fn new(api_client: Arc<ApiClient>) -> Self {
        Self { api_client }
    }

    pub async fn start(self) {
        let mut interval = tokio::time::interval(Duration::from_secs(1));
        interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
        loop {
            interval.tick().await;
            
            // 1. Fetch
            let stats = match self.api_client.get_monitor_stats().await {
                Ok(s) => s,
                Err(_) => continue,
            };

            // 2. Compute & Update
            let mut data = MONITOR_DATA.write().await;
            let mut point = MonitorPoint {
                traffic_in_bps: 0,
                traffic_out_bps: 0,
            };

            if let Some(last) = &data.last_raw {
                point.traffic_in_bps = stats.audio_traffic_in.saturating_sub(last.audio_traffic_in);
                point.traffic_out_bps = stats.audio_traffic_out.saturating_sub(last.audio_traffic_out);
            }

            data.last_raw = Some(stats);
            data.history.push_back(point);
            if data.history.len() > 100 {
                data.history.pop_front();
            }
        }
    }
}
