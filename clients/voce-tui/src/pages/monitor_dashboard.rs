use std::sync::Arc;

use crossterm::event::KeyCode;
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph, Row, Sparkline, Table},
    Frame,
};
use tokio::sync::mpsc;

use crate::api::ApiClient;
use crate::common::events::Event;
use crate::monitor::{MonitorData, MonitorMode, MonitorStatus, MONITOR_DATA};
use crate::pages::{Page, PageResponse};

pub struct MonitorDashboard {
    api_client: Arc<ApiClient>,
    event_tx: mpsc::Sender<Event>,
}

impl MonitorDashboard {
    pub fn new(api_client: Arc<ApiClient>, event_tx: mpsc::Sender<Event>) -> Self {
        Self { api_client, event_tx }
    }

    fn back(&self) -> PageResponse {
        PageResponse::SwitchTo(Box::new(super::workflow_list::WorkflowList::new(
            self.api_client.clone(),
            self.event_tx.clone(),
        )))
    }
}

impl Page for MonitorDashboard {
    fn handle_event(&mut self, event: &Event) -> PageResponse {
        if let Event::Input(key) = event {
            match key.code {
                KeyCode::Esc | KeyCode::Char('q') => return self.back(),
                _ => {}
            }
        }
        PageResponse::None
    }

    fn draw(&mut self, frame: &mut Frame) {
        let area = frame.area();
        let Ok(data) = MONITOR_DATA.try_read() else {
            frame.render_widget(
                Paragraph::new(" monitor data is busy; retrying...")
                    .block(panel(" MONITOR ", Color::Yellow)),
                area,
            );
            return;
        };

        let machine_height = data
            .last_snapshot
            .as_ref()
            .and_then(|snapshot| snapshot.gateway_state.as_ref())
            .filter(|_| area.height >= 24)
            .map(|state| (state.machine_states.len() as u16 + 3).clamp(5, 8))
            .unwrap_or(0);

        let vertical = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(4),
                Constraint::Min(12),
                Constraint::Length(machine_height),
                Constraint::Length(1),
            ])
            .split(area);

        draw_header(frame, vertical[0], &data);

        if area.width < 96 {
            draw_compact_dashboard(frame, vertical[1], &data);
        } else {
            draw_wide_dashboard(frame, vertical[1], &data);
        }

        if machine_height > 0 {
            draw_machines(frame, vertical[2], &data);
        }

        frame.render_widget(
            Paragraph::new(" [q/Esc] back   refresh: 1s   values are sampled from /metrics"),
            vertical[3],
        );
    }
}

fn draw_header(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let (mode, status, age, target, service, environment, error) = match &data.last_snapshot {
        Some(snapshot) => {
            let mode = match snapshot.mode {
                MonitorMode::Standalone => "standalone",
                MonitorMode::Gateway => "gateway",
            };
            let status = match snapshot.status {
                MonitorStatus::Connected => ("CONNECTED", Color::Green),
                MonitorStatus::Stale => ("STALE", Color::Yellow),
            };
            let age = snapshot.collected_at.elapsed().as_secs_f64();
            (
                mode,
                status,
                age,
                snapshot.target.as_str(),
                snapshot.service_name.as_deref().unwrap_or("unknown"),
                snapshot.environment.as_deref().unwrap_or("unknown"),
                data.last_error.as_deref(),
            )
        }
        None => (
            "detecting",
            ("WAITING", Color::Yellow),
            0.0,
            "-",
            "-",
            "-",
            data.last_error.as_deref(),
        ),
    };

    let text = vec![Line::from(vec![
        Span::styled(" VOCE MONITOR ", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
        Span::raw("  mode: "),
        Span::styled(mode, Style::default().fg(Color::White)),
        Span::raw("  service: "),
        Span::styled(service, Style::default().fg(Color::White)),
        Span::raw("/"),
        Span::styled(environment, Style::default().fg(Color::DarkGray)),
        Span::raw("  "),
        Span::styled(status.0, Style::default().fg(status.1).add_modifier(Modifier::BOLD)),
        Span::raw(format!("  updated: {:.1}s ago", age)),
    ]), Line::from(vec![
        Span::raw(" target: "),
        Span::styled(target, Style::default().fg(Color::White)),
        Span::raw(format!(
            "  window: {}s  samples: {}",
            data.history.len(),
            data.history.len()
        )),
        Span::styled(
            error.map_or_else(String::new, |value| format!("  error: {value}")),
            Style::default().fg(Color::Red),
        ),
    ])];
    frame.render_widget(Paragraph::new(text).block(panel(" status ", Color::Cyan)), area);
}

fn draw_wide_dashboard(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
        .split(area);
    let top = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
        .split(rows[0]);
    let bottom = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
        .split(rows[1]);

    draw_traffic(frame, top[0], data);
    draw_sessions(frame, top[1], data);
    draw_runtime(frame, bottom[0], data);
    draw_pool(frame, bottom[1], data);
}

fn draw_compact_dashboard(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Percentage(45), Constraint::Percentage(28), Constraint::Percentage(27)])
        .split(area);
    draw_traffic(frame, rows[0], data);

    let metrics = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
        .split(rows[1]);
    draw_sessions(frame, metrics[0], data);
    draw_runtime(frame, metrics[1], data);
    draw_pool(frame, rows[2], data);
}

fn draw_traffic(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let rx = values(data, |point| point.traffic_rx);
    let tx = values(data, |point| point.traffic_tx);
    let current = data
        .last_snapshot
        .as_ref()
        .map(|s| (s.traffic.rx_bytes_per_second, s.traffic.tx_bytes_per_second));
    let (rx_rate, tx_rate) = current
        .map(|(rx_now, tx_now)| (rate(rx_now), rate(tx_now)))
        .unwrap_or_else(|| ("waiting".to_owned(), "waiting".to_owned()));
    let title = Line::from(vec![
        Span::styled(" traffic  ", Style::default().fg(Color::Gray)),
        Span::styled("RX ", Style::default().fg(Color::Green).add_modifier(Modifier::BOLD)),
        Span::styled(rx_rate, Style::default().fg(Color::Green)),
        Span::raw("  "),
        Span::styled("TX ", Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)),
        Span::styled(tx_rate, Style::default().fg(Color::Yellow)),
        Span::raw(" "),
    ]);
    let block = Block::default()
        .title(title)
        .borders(Borders::ALL)
        .border_style(Style::default().fg(Color::Gray));
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let padded = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Length(1),
            Constraint::Min(1),
            Constraint::Length(1),
        ])
        .split(inner)[1];
    let columns = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
        .split(padded);
    frame.render_widget(
        Sparkline::default()
            .data(rx)
            .style(Style::default().fg(Color::Green)),
        columns[0],
    );
    frame.render_widget(
        Sparkline::default()
            .data(tx)
            .style(Style::default().fg(Color::Yellow)),
        columns[1],
    );
}

fn draw_sessions(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let active_history = values(data, |point| point.sessions);
    let active_min = active_history.iter().copied().min().unwrap_or(0);
    let active_max = active_history.iter().copied().max().unwrap_or(0);
    let snapshot = data.last_snapshot.as_ref();
    let current = snapshot.map(|s| s.sessions.active as i64).unwrap_or(0);
    let routed = snapshot.and_then(|s| s.sessions.routed).map(|value| value as i64);
    let client_connections = snapshot
        .and_then(|s| s.client_connections)
        .map(|value| value as i64);
    let title = format!(" sessions  total {active_min}-{active_max} ");
    let block = panel(&title, Color::Cyan);
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let padded = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Length(1),
            Constraint::Min(1),
            Constraint::Length(1),
        ])
        .split(inner)[1];
    let rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(1), Constraint::Length(1), Constraint::Min(1)])
        .split(padded);
    let metrics = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage(33),
            Constraint::Percentage(34),
            Constraint::Percentage(33),
        ])
        .split(rows[0]);
    draw_metric_cell(frame, metrics[0], "Total", &current.to_string());
    draw_metric_cell(
        frame,
        metrics[1],
        "Client conns",
        &optional_count(client_connections),
    );
    draw_metric_cell(frame, metrics[2], "Routed", &optional_count(routed));

    frame.render_widget(
        Sparkline::default()
            .data(active_history)
            .style(Style::default().fg(Color::Cyan)),
        rows[2],
    );
}

fn draw_runtime(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let snapshot = data.last_snapshot.as_ref();
    let goroutines = snapshot.map(|s| s.runtime.goroutines as i64).unwrap_or(0);
    let heap_alloc = snapshot.map(|s| s.runtime.heap_alloc_bytes / 1024.0 / 1024.0).unwrap_or(0.0);
    let heap_inuse = snapshot.map(|s| s.runtime.heap_inuse_bytes / 1024.0 / 1024.0).unwrap_or(0.0);
    let gc_count = snapshot.map(|s| s.runtime.gc_count as i64).unwrap_or(0);
    let gc_pause = snapshot.map(|s| s.runtime.gc_pause_ns_per_second / 1_000_000.0).unwrap_or(0.0);
    let heap_history = values(data, |point| point.heap_alloc_bytes / 1024.0);
    let heap_values: Vec<f64> = data
        .history
        .iter()
        .map(|point| point.heap_alloc_bytes / 1024.0 / 1024.0)
        .collect();
    let heap_min = heap_values.iter().copied().reduce(f64::min).unwrap_or(0.0);
    let heap_max = heap_values.iter().copied().reduce(f64::max).unwrap_or(0.0);

    let title = format!(" runtime  heap {:.1}-{:.1} MiB ", heap_min, heap_max);
    let block = panel(&title, Color::Magenta);
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let padded = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Length(1),
            Constraint::Min(1),
            Constraint::Length(1),
        ])
        .split(inner)[1];
    let rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(2), Constraint::Length(1), Constraint::Min(1)])
        .split(padded);
    let metric_rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(1), Constraint::Length(1)])
        .split(rows[0]);
    let heap_columns = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
        .split(metric_rows[0]);
    draw_metric_cell(frame, heap_columns[0], "Heap alloc", &format!("{heap_alloc:.1} MiB"));
    draw_metric_cell(frame, heap_columns[1], "Heap inuse", &format!("{heap_inuse:.1} MiB"));

    let runtime_columns = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage(33),
            Constraint::Percentage(33),
            Constraint::Percentage(34),
        ])
        .split(metric_rows[1]);
    draw_metric_cell(frame, runtime_columns[0], "Goroutines", &goroutines.to_string());
    draw_metric_cell(frame, runtime_columns[1], "GC count", &gc_count.to_string());
    draw_metric_cell(frame, runtime_columns[2], "GC pause", &format!("{gc_pause:.2}ms/s"));

    frame.render_widget(
        Sparkline::default()
            .data(heap_history)
            .style(Style::default().fg(Color::Cyan)),
        rows[2],
    );
}

fn draw_metric_cell(frame: &mut Frame, area: Rect, label: &str, value: &str) {
    frame.render_widget(
        Paragraph::new(Line::from(vec![
            Span::styled(label.to_owned(), Style::default().fg(Color::Gray)),
            Span::raw(" "),
            Span::styled(value.to_owned(), Style::default().fg(Color::White)),
        ])),
        area,
    );
}

fn draw_pool(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let snapshot = data.last_snapshot.as_ref();
    let (active, connecting, pending, routed, reconnects, errors) = snapshot
        .map(|s| (s.pool.active as i64, s.pool.connecting as i64, s.pool.pending_dials as i64, s.pool.sessions_routed as i64, s.pool.reconnects_per_second, s.pool.write_errors_per_second))
        .unwrap_or_default();
    let active_history = values(data, |point| point.pool_active);
    let active_min = active_history.iter().copied().min().unwrap_or(0);
    let active_max = active_history.iter().copied().max().unwrap_or(0);
    let title = format!(" pool  active {active_min}-{active_max} ");
    let block = panel(&title, Color::Yellow);
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let padded = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Length(1),
            Constraint::Min(1),
            Constraint::Length(1),
        ])
        .split(inner)[1];
    let rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(2), Constraint::Length(1), Constraint::Min(1)])
        .split(padded);
    let metric_rows = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(1), Constraint::Length(1)])
        .split(rows[0]);
    let first_row = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage(33),
            Constraint::Percentage(34),
            Constraint::Percentage(33),
        ])
        .split(metric_rows[0]);
    draw_metric_cell(frame, first_row[0], "Active", &active.to_string());
    draw_metric_cell(frame, first_row[1], "Connecting", &connecting.to_string());
    draw_metric_cell(frame, first_row[2], "Pending", &pending.to_string());

    let second_row = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage(33),
            Constraint::Percentage(34),
            Constraint::Percentage(33),
        ])
        .split(metric_rows[1]);
    draw_metric_cell(frame, second_row[0], "Routed", &routed.to_string());
    draw_metric_cell(frame, second_row[1], "Reconnect", &format!("{reconnects:.2}/s"));
    draw_metric_cell(frame, second_row[2], "Write err", &format!("{errors:.2}/s"));

    frame.render_widget(
        Sparkline::default()
            .data(active_history)
            .style(Style::default().fg(Color::Yellow)),
        rows[2],
    );
}

fn draw_machines(frame: &mut Frame, area: Rect, data: &MonitorData) {
    let Some(snapshot) = data.last_snapshot.as_ref() else { return; };
    let Some(state) = snapshot.gateway_state.as_ref() else { return; };
    let rows = state.machine_states.iter().map(|machine| {
        Row::new(vec![
            short_machine_id(&machine.id),
            machine.address.clone(),
            machine.sessions.to_string(),
            format!("{}  hb {}", machine_state_name(machine.state), heartbeat_age(machine.last_heartbeat)),
        ])
    });
    let table = Table::new(rows, [Constraint::Percentage(24), Constraint::Percentage(34), Constraint::Percentage(18), Constraint::Percentage(24)])
        .header(Row::new(vec!["machine", "address", "sessions", "state / heartbeat"]).style(Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)))
        .block(panel(
            &format!(
                " machines  {}/{} active  gateway sessions {} ",
                state.active_machines, state.machines, state.sessions
            ),
            Color::Blue,
        ));
    frame.render_widget(table, area);
}

fn heartbeat_age(timestamp_ms: i64) -> String {
    if timestamp_ms <= 0 {
        return "-".to_owned();
    }
    let now_ms = chrono::Utc::now().timestamp_millis();
    let age = (now_ms - timestamp_ms).max(0) / 1000;
    format!("{}s", age)
}

fn machine_state_name(state: i32) -> &'static str {
    match state {
        1 => "active",
        2 => "suspended",
        3 => "terminated",
        _ => "unknown",
    }
}

fn short_machine_id(id: &str) -> String {
    id.split('-').next().unwrap_or(id).chars().take(8).collect()
}

fn optional_count(value: Option<i64>) -> String {
    value.map_or_else(|| "--".to_owned(), |value| value.to_string())
}

fn values(data: &MonitorData, selector: impl Fn(&crate::monitor::MonitorPoint) -> f64) -> Vec<u64> {
    data.history.iter().map(selector).map(|value| value.max(0.0) as u64).collect()
}

fn rate(value: f64) -> String {
    const KILO: f64 = 1_000.0;
    const MEGA: f64 = 1_000_000.0;
    const GIGA: f64 = 1_000_000_000.0;

    if value >= GIGA {
        format!("{:.1}GB/s", value / GIGA)
    } else if value >= MEGA {
        format!("{:.1}MB/s", value / MEGA)
    } else if value >= KILO {
        format!("{:.1}KB/s", value / KILO)
    } else {
        format!("{value:.0}B/s")
    }
}

fn panel(title: &str, color: Color) -> Block<'static> {
    Block::default().title(title.to_owned()).borders(Borders::ALL).border_style(Style::default().fg(color))
}

#[cfg(test)]
mod tests {
    use super::rate;

    #[test]
    fn formats_network_rate_with_decimal_units() {
        assert_eq!(rate(999.0), "999B/s");
        assert_eq!(rate(1_000.0), "1.0KB/s");
        assert_eq!(rate(1_500_000.0), "1.5MB/s");
        assert_eq!(rate(1_250_000_000.0), "1.2GB/s");
    }
}
