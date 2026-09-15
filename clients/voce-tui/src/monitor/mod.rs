use std::collections::{HashMap, VecDeque};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::Result;
use once_cell::sync::Lazy;
use tokio::sync::RwLock;
use tracing::warn;

use crate::api::{ApiClient, GatewayState};

const HISTORY_LENGTH: usize = 120;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MonitorMode {
    Standalone,
    Gateway,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MonitorStatus {
    Connected,
    Stale,
}

#[derive(Debug, Clone, Default)]
pub struct RuntimeMetrics {
    pub goroutines: f64,
    pub heap_alloc_bytes: f64,
    pub heap_inuse_bytes: f64,
    pub gc_count: f64,
    pub gc_pause_ns_per_second: f64,
}

#[derive(Debug, Clone, Default)]
pub struct SessionMetrics {
    pub active: f64,
    pub routed: Option<f64>,
}

#[derive(Debug, Clone, Default)]
pub struct TrafficMetrics {
    pub rx_bytes_per_second: f64,
    pub tx_bytes_per_second: f64,
}

#[derive(Debug, Clone, Default)]
pub struct PoolMetrics {
    pub active: f64,
    pub connecting: f64,
    pub pending_dials: f64,
    pub sessions_routed: f64,
    pub reconnects_per_second: f64,
    pub write_errors_per_second: f64,
}

#[derive(Debug, Clone)]
pub struct MonitorSnapshot {
    pub mode: MonitorMode,
    pub status: MonitorStatus,
    pub target: String,
    pub service_name: Option<String>,
    pub environment: Option<String>,
    pub collected_at: Instant,
    pub runtime: RuntimeMetrics,
    pub sessions: SessionMetrics,
    pub client_connections: Option<f64>,
    pub traffic: TrafficMetrics,
    pub pool: PoolMetrics,
    pub gateway_state: Option<GatewayState>,
}

#[derive(Debug, Clone, Default)]
pub struct MonitorPoint {
    pub traffic_rx: f64,
    pub traffic_tx: f64,
    pub sessions: f64,
    pub heap_alloc_bytes: f64,
    pub pool_active: f64,
}

pub struct MonitorData {
    pub history: VecDeque<MonitorPoint>,
    pub last_snapshot: Option<MonitorSnapshot>,
    pub last_error: Option<String>,
}

pub static MONITOR_DATA: Lazy<Arc<RwLock<MonitorData>>> = Lazy::new(|| {
    Arc::new(RwLock::new(MonitorData {
        history: VecDeque::with_capacity(HISTORY_LENGTH),
        last_snapshot: None,
        last_error: None,
    }))
});

#[derive(Default)]
struct PreviousCounters {
    values: HashMap<String, f64>,
    at: Option<Instant>,
}

pub struct MonitorWorker {
    api_client: Arc<ApiClient>,
    previous: PreviousCounters,
}

impl MonitorWorker {
    pub fn new(api_client: Arc<ApiClient>) -> Self {
        Self {
            api_client,
            previous: PreviousCounters::default(),
        }
    }

    pub async fn start(mut self) {
        let mut interval = tokio::time::interval(Duration::from_secs(1));
        interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
        loop {
            interval.tick().await;
            if let Err(error) = self.collect().await {
                warn!("monitor collection failed: {error}");
                let mut data = MONITOR_DATA.write().await;
                data.last_error = Some(error.to_string());
                if let Some(snapshot) = data.last_snapshot.as_mut() {
                    snapshot.status = MonitorStatus::Stale;
                }
            }
        }
    }

    async fn collect(&mut self) -> Result<()> {
        let now = Instant::now();
        let raw = self.api_client.get_metrics().await?;
        let metrics = parse_metrics(&raw);
        let service_name = metric_label(&raw, "target_info", "service_name");
        let environment = metric_label(
            &raw,
            "target_info",
            "deployment_environment_name",
        );
        let mut gateway_state = self.api_client.get_gateway_state().await.ok();
        if let Some(state) = gateway_state.as_mut() {
            state.machine_states.sort_by(|left, right| left.id.cmp(&right.id));
        }
        let mode = if gateway_state.is_some() {
            MonitorMode::Gateway
        } else {
            MonitorMode::Standalone
        };
        let elapsed = self
            .previous
            .at
            .map(|at| now.duration_since(at).as_secs_f64())
            .filter(|v| *v > 0.0)
            .unwrap_or(1.0);

        let runtime = RuntimeMetrics {
            goroutines: metric_value(&metrics, "voce_process_runtime_go_goroutines", None),
            heap_alloc_bytes: metric_value(
                &metrics,
                "voce_process_runtime_go_mem_heap_alloc_bytes",
                None,
            ),
            heap_inuse_bytes: metric_value(
                &metrics,
                "voce_process_runtime_go_mem_heap_inuse_bytes",
                None,
            ),
            gc_count: metric_value(&metrics, "voce_process_runtime_go_gc_count_total", None),
            gc_pause_ns_per_second: counter_rate(
                &metrics,
                &mut self.previous,
                "runtime.gc.pause",
                elapsed,
                &["voce_process_runtime_go_gc_pause_ns_total"],
            ),
        };
        let session_scope = match mode {
            MonitorMode::Standalone => "github.com/wnnce/voce/internal/engine",
            MonitorMode::Gateway => "github.com/wnnce/voce/internal/gateway",
        };
        let sessions = SessionMetrics {
            active: metric_value(
                &metrics,
                "voce_sessions_active",
                Some(session_scope),
            ),
            routed: match mode {
                MonitorMode::Gateway => Some(metric_value(
                    &metrics,
                    "voce_gateway_pool_sessions_routed",
                    None,
                )),
                MonitorMode::Standalone => None,
            },
        };
        let traffic = TrafficMetrics {
            rx_bytes_per_second: counter_rate(
                &metrics,
                &mut self.previous,
                "rx.bytes",
                elapsed,
                &[
                    "voce_realtime_websocket_bytes_received_total",
                    "voce_gateway_client_websocket_bytes_received_total",
                ],
            ),
            tx_bytes_per_second: counter_rate(
                &metrics,
                &mut self.previous,
                "tx.bytes",
                elapsed,
                &[
                    "voce_realtime_websocket_bytes_sent_total",
                    "voce_gateway_client_websocket_bytes_sent_total",
                ],
            ),
        };
        let pool = PoolMetrics {
            active: metric_value(&metrics, "voce_gateway_pool_connections_active", None),
            connecting: metric_value(&metrics, "voce_gateway_pool_connections_connecting", None),
            pending_dials: metric_value(&metrics, "voce_gateway_pool_pending_dials", None),
            sessions_routed: metric_value(&metrics, "voce_gateway_pool_sessions_routed", None),
            reconnects_per_second: counter_rate(
                &metrics,
                &mut self.previous,
                "reconnects",
                elapsed,
                &["voce_gateway_pool_reconnects_total"],
            ),
            write_errors_per_second: counter_rate(
                &metrics,
                &mut self.previous,
                "write.errors",
                elapsed,
                &["voce_gateway_pool_write_errors_total"],
            ),
        };
        let client_connections = gateway_state
            .as_ref()
            .map(|state| state.client_connections as f64);
        let snapshot = MonitorSnapshot {
            mode,
            status: MonitorStatus::Connected,
            target: self.api_client.get_base_url().to_owned(),
            service_name,
            environment,
            collected_at: now,
            runtime,
            sessions,
            client_connections,
            traffic: traffic.clone(),
            pool: pool.clone(),
            gateway_state,
        };
        let point = MonitorPoint {
            traffic_rx: traffic.rx_bytes_per_second,
            traffic_tx: traffic.tx_bytes_per_second,
            sessions: snapshot.sessions.active,
            heap_alloc_bytes: snapshot.runtime.heap_alloc_bytes,
            pool_active: pool.active,
        };
        self.previous.at = Some(now);

        let mut data = MONITOR_DATA.write().await;
        data.history.push_back(point);
        if data.history.len() > HISTORY_LENGTH {
            data.history.pop_front();
        }
        data.last_snapshot = Some(snapshot);
        data.last_error = None;
        Ok(())
    }
}

type MetricMap = HashMap<(String, Option<String>), f64>;

fn parse_metrics(body: &str) -> MetricMap {
    let mut metrics = MetricMap::new();
    for line in body.lines() {
        let mut parts = line.split_whitespace();
        let Some(name_part) = parts.next() else {
            continue;
        };
        let Some(value_part) = parts.next() else {
            continue;
        };
        let Some(value) = value_part.parse::<f64>().ok() else {
            continue;
        };
        let (name, scope) = match name_part.find('{') {
            Some(start) => {
                let labels = &name_part[start..];
                let scope = labels
                    .split("otel_scope_name=\"")
                    .nth(1)
                    .and_then(|v| v.split('"').next())
                    .map(str::to_owned);
                (&name_part[..start], scope)
            }
            None => (name_part, None),
        };
        metrics.insert((name.to_owned(), scope), value);
    }
    metrics
}

fn metric_value(metrics: &MetricMap, name: &str, scope: Option<&str>) -> f64 {
    match scope {
        Some(scope) => metrics
            .get(&(name.to_owned(), Some(scope.to_owned())))
            .copied()
            .unwrap_or(0.0),
        None => metrics
            .iter()
            .filter(|((metric_name, _), _)| metric_name == name)
            .map(|(_, value)| *value)
            .sum(),
    }
}

fn metric_label(body: &str, metric_name: &str, label_name: &str) -> Option<String> {
    let prefix = format!("{metric_name}{{");
    let label_prefix = format!("{label_name}=\"");
    body.lines()
        .find(|line| line.starts_with(&prefix))
        .and_then(|line| line.split(&label_prefix).nth(1))
        .and_then(|value| value.split('"').next())
        .map(str::to_owned)
}

fn counter_rate(
    metrics: &MetricMap,
    previous: &mut PreviousCounters,
    key: &str,
    elapsed: f64,
    names: &[&str],
) -> f64 {
    let name = names
        .iter()
        .find(|name| metrics.keys().any(|(metric, _)| metric == **name));
    let Some(name) = name else {
        return 0.0;
    };
    let current = metric_value(metrics, name, None);
    let previous_value = previous
        .values
        .insert(key.to_owned(), current)
        .unwrap_or(current);
    (current - previous_value).max(0.0) / elapsed
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_metric_values_and_scope() {
        let metrics = parse_metrics(
            "# HELP voce_sessions_active active sessions\nvoce_sessions_active{otel_scope_name=\"engine\"} 7\nvoce_gateway_pool_connections_active 3\n",
        );

        assert_eq!(metric_value(&metrics, "voce_sessions_active", Some("engine")), 7.0);
        assert_eq!(metric_value(&metrics, "voce_gateway_pool_connections_active", None), 3.0);
        assert_eq!(metric_value(&metrics, "voce_sessions_active", Some("gateway")), 0.0);
        assert_eq!(metric_value(&metrics, "voce_sessions_active", None), 7.0);
    }

    #[test]
    fn parses_resource_labels() {
        let metrics = "target_info{deployment_environment_name=\"dev\",service_name=\"voce\"} 1\n";

        assert_eq!(metric_label(metrics, "target_info", "service_name").as_deref(), Some("voce"));
        assert_eq!(
            metric_label(metrics, "target_info", "deployment_environment_name").as_deref(),
            Some("dev")
        );
    }

    #[test]
    fn converts_counter_delta_to_rate_and_clamps_reset() {
        let mut previous = PreviousCounters::default();
        let first = parse_metrics("voce_bytes_total 100\n");
        let second = parse_metrics("voce_bytes_total 160\n");
        let reset = parse_metrics("voce_bytes_total 10\n");

        assert_eq!(counter_rate(&first, &mut previous, "bytes", 1.0, &["voce_bytes_total"]), 0.0);
        assert_eq!(counter_rate(&second, &mut previous, "bytes", 2.0, &["voce_bytes_total"]), 30.0);
        assert_eq!(counter_rate(&reset, &mut previous, "bytes", 1.0, &["voce_bytes_total"]), 0.0);
    }
}
