use ratatui::{
    layout::{Constraint, Direction, Layout},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph, Chart, Dataset, Axis},
    symbols::Marker,
    Frame,
};
use std::sync::Arc;
use tokio::sync::mpsc;
use crate::common::events::Event;
use crate::pages::{Page, PageResponse};
use crate::monitor::MONITOR_DATA;
use crate::api::ApiClient;

pub struct MonitorDashboard {
    api_client: Arc<ApiClient>,
    event_tx: mpsc::Sender<Event>,
}

impl MonitorDashboard {
    pub fn new(api_client: Arc<ApiClient>, event_tx: mpsc::Sender<Event>) -> Self {
        Self { api_client, event_tx }
    }
}

impl Page for MonitorDashboard {
    fn handle_event(&mut self, event: &Event) -> PageResponse {
        if let Event::Input(key) = event {
            use crossterm::event::KeyCode;
            match key.code {
                KeyCode::Esc | KeyCode::Char('q') => {
                    return PageResponse::SwitchTo(Box::new(super::workflow_list::WorkflowList::new(self.api_client.clone(), self.event_tx.clone())));
                }
                _ => {}
            }
        }
        PageResponse::None
    }

    fn draw(&mut self, f: &mut Frame) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(3), // Header
                Constraint::Min(10),   // Full-width Traffic Chart
                Constraint::Length(10), // Bottom Stats (Mem, GC, etc)
            ])
            .split(f.size());

        // 1. Header
        f.render_widget(Paragraph::new(" [ESC/q] Back to List | SYSTEM WIDE MONITORING ").block(Block::default().borders(Borders::ALL).border_style(Style::default().fg(Color::Cyan))), chunks[0]);

        // Access data
        let Ok(data) = MONITOR_DATA.try_read() else { return };
        let mut traffic_in: Vec<f64> = data.history.iter().map(|p| p.traffic_in_bps as f64).collect();
        let mut traffic_out: Vec<f64> = data.history.iter().map(|p| p.traffic_out_bps as f64).collect();

        // Ensure we have some data to draw
        if traffic_in.is_empty() { 
            traffic_in.push(0.0); traffic_out.push(0.0);
        }

        // 2. Large Traffic Chart (Merged In/Out)
        let in_data: Vec<(f64, f64)> = traffic_in.iter().enumerate().map(|(i, &v)| (i as f64, v)).collect();
        let out_data: Vec<(f64, f64)> = traffic_out.iter().enumerate().map(|(i, &v)| (i as f64, v)).collect();

        let max_val = traffic_in.iter().cloned().chain(traffic_out.iter().cloned()).fold(1.0, f64::max).max(1024.0);

        let datasets = vec![
            Dataset::default()
                .name("Traffic IN (Bps)")
                .marker(Marker::Braille)
                .style(Style::default().fg(Color::Green))
                .data(&in_data),
            Dataset::default()
                .name("Traffic OUT (Bps)")
                .marker(Marker::Dot)
                .style(Style::default().fg(Color::Yellow))
                .data(&out_data),
        ];

        let chart = Chart::new(datasets)
            .block(Block::default().title(" Real-time Network Throughput ").borders(Borders::ALL))
            .x_axis(Axis::default().title("Time (s)").style(Style::default().fg(Color::Gray)).bounds([0.0, 100.0]))
            .y_axis(Axis::default().title("Bytes/s").style(Style::default().fg(Color::Gray)).bounds([0.0, max_val]).labels(vec![
                Span::raw("0"),
                Span::raw(format!("{:.1} KB/s", max_val / 2048.0)),
                Span::raw(format!("{:.1} KB/s", max_val / 1024.0)),
            ]));
        f.render_widget(chart, chunks[1]);

        // 3. Bottom Grid (Three Columns)
        let bottom_chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([
                Constraint::Percentage(33), // Memory
                Constraint::Percentage(34), // Connectivity & Runtime
                Constraint::Percentage(33), // Business Resource Pools
            ])
            .split(chunks[2]);

        if let Some(last) = &data.last_raw {
            // Memory Panel
            let mem_text = vec![
                Line::from(vec![Span::raw(" Heap Alloc:  "), Span::styled(format!("{} MB", last.heap_alloc / 1024 / 1024), Style::default().fg(Color::Cyan))]),
                Line::from(vec![Span::raw(" Heap Inuse:  "), Span::styled(format!("{} MB", last.heap_inuse / 1024 / 1024), Style::default().fg(Color::Yellow))]),
                Line::from(vec![Span::raw(" System Tot:  "), Span::styled(format!("{} MB", last.system_mem / 1024 / 1024), Style::default().fg(Color::Magenta))]),
                Line::from(vec![Span::raw(" GC Pause:    "), Span::styled(format!("{} ms", last.pause_total_ns / 1_000_000), Style::default().fg(Color::Red))]),
            ];
            f.render_widget(Paragraph::new(mem_text).block(Block::default().title(" Memory Details ").borders(Borders::ALL)), bottom_chunks[0]);

            // Connectivity & Runtime Panel
            let conn_text = vec![
                Line::from(vec![Span::raw(" Sessions:    "), Span::styled(last.active_sessions.to_string(), Style::default().fg(Color::Green).add_modifier(Modifier::BOLD))]),
                Line::from(vec![Span::raw(" Connections: "), Span::styled(last.active_connections.to_string(), Style::default().fg(Color::Cyan))]),
                Line::from(vec![Span::raw(" Goroutines:  "), Span::styled(last.goroutines.to_string(), Style::default().fg(Color::Blue))]),
                Line::from(vec![Span::raw(" Num GC:      "), Span::styled(last.num_gc.to_string(), Style::default().fg(Color::DarkGray))]),
            ];
            f.render_widget(Paragraph::new(conn_text).block(Block::default().title(" Connectivity & Hub ").borders(Borders::ALL)), bottom_chunks[1]);

            // Business Resource Pools Panel
            let pool_text = vec![
                Line::from(vec![Span::raw(" Audio    : "), Span::styled(last.active_audio_count.to_string(), Style::default().fg(Color::Magenta))]),
                Line::from(vec![Span::raw(" SD Video : "), Span::styled(last.active_sd_video_count.to_string(), Style::default().fg(Color::Gray))]),
                Line::from(vec![Span::raw(" HD Video : "), Span::styled(last.active_hd_video_count.to_string(), Style::default().fg(Color::Yellow))]),
                Line::from(vec![Span::raw(" FHD Video: "), Span::styled(last.active_fhd_video_count.to_string(), Style::default().fg(Color::Red))]),
            ];
            f.render_widget(Paragraph::new(pool_text).block(Block::default().title(" Resource Pools ").borders(Borders::ALL)), bottom_chunks[2]);
        }
    }
}
