use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph, Wrap},
    Frame,
};
use unicode_width::UnicodeWidthStr;
use std::collections::VecDeque;
use std::sync::Arc;
use tokio::sync::{mpsc, oneshot};
use crate::common::events::Event;
use crate::pages::{Page, PageResponse};
use crate::session::{SessionManager, SessionEvent};
use crate::protocol::{Packet, PacketType};
use tracing::{info, error, debug};
use crate::audio::AudioHandle;
use crate::api::ApiClient;
use crate::monitor::MONITOR_DATA;

pub struct Subtitle {
    pub role: String,
    pub text: String,
    pub is_final: bool,
    pub cached_lines: Vec<Line<'static>>,
}

pub struct Chat {
    workflow_name: String,
    properties: serde_json::Value,
    api_client: Arc<ApiClient>,
    event_tx: mpsc::Sender<Event>,
    current_handle: Option<Arc<AudioHandle>>,
    stop_tx: Option<oneshot::Sender<bool>>,
    user_speaking: bool,
    agent_speaking: bool,
    agent_playback_active: bool,
    subtitles: VecDeque<Subtitle>,
    
    // Professional Visualizer State
    visualizer_data: Vec<f32>,   // Smoothed heights (0.0 - 1.0)
    energy_window: VecDeque<f32>, // Still used for Idle detection
    initialized: bool,
    
    // Scrolling
    scroll_offset: u16,
    auto_scroll: bool,
    error_message: Option<String>,

    // Session Persistence & Pause Support
    session_id: Option<String>,
    is_paused: bool,
    heartbeat_task: Option<tokio::task::JoinHandle<()>>,
}

impl Drop for Chat {
    fn drop(&mut self) {
        self.stop_heartbeat();
        if let Some(tx) = self.stop_tx.take() {
            let _ = tx.send(true); // true = Shutdown
        }
    }
}

impl Chat {
    pub fn new(workflow_name: String, properties: serde_json::Value, api_client: Arc<ApiClient>, event_tx: mpsc::Sender<Event>) -> Self {
        Self {
            workflow_name, properties, api_client, event_tx, current_handle: None, stop_tx: None, user_speaking: false, agent_speaking: false, agent_playback_active: false, subtitles: VecDeque::with_capacity(50), 
            visualizer_data: vec![0.0; 128], 
            energy_window: VecDeque::with_capacity(8), 
            initialized: false,
            scroll_offset: 0,
            auto_scroll: true,
            error_message: None,
            session_id: None,
            is_paused: false,
            heartbeat_task: None,
        }
    }

    fn init_session(&mut self) {
        if self.initialized || self.is_paused { return; }
        self.initialized = true;
        
        let (session_tx, mut session_rx) = mpsc::channel(50);
        let (stop_tx, stop_rx) = oneshot::channel();
        self.stop_tx = Some(stop_tx);
        
        let manager = SessionManager::new(self.api_client.clone(), session_tx);
        let workflow = self.workflow_name.clone();
        let props = self.properties.clone();
        let session_id = self.session_id.clone();
        let tx = self.event_tx.clone();
        
        tokio::spawn(async move {
            info!("UI: Starting Session Manager task for workflow: {}", workflow);
            manager.start(workflow, props, session_id, stop_rx).await;
            info!("UI: Session Manager task terminated.");
        });

        tokio::spawn(async move {
            while let Some(se) = session_rx.recv().await {
                if tx.send(Event::SessionUpdate(se)).await.is_err() { break; }
            }
        });
    }

    fn toggle_pause(&mut self) {
        self.is_paused = !self.is_paused;
        if self.is_paused {
            info!("UI: Pausing session...");
            if let Some(tx) = self.stop_tx.take() {
                let _ = tx.send(false); // false = Pause (don't send Close packet)
            }
            self.start_heartbeat();
            self.agent_speaking = false;
            self.user_speaking = false;
            self.agent_playback_active = false;
            self.current_handle = None;
        } else {
            info!("UI: Resuming session: {:?}", self.session_id);
            self.stop_heartbeat();
            self.initialized = false;
            self.error_message = None;
        }
    }

    fn start_heartbeat(&mut self) {
        if self.heartbeat_task.is_some() { return; }
        let id = match &self.session_id { Some(id) => id.clone(), None => return };
        let api = self.api_client.clone();
        
        self.heartbeat_task = Some(tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(10));
            loop {
                interval.tick().await;
                if let Err(e) = api.renew_session(&id).await {
                    error!("UI: Session heartbeat failed: {}", e);
                    break;
                }
                debug!("UI: Session heartbeat success for {}", id);
            }
        }));
    }

    fn stop_heartbeat(&mut self) {
        if let Some(task) = self.heartbeat_task.take() {
            task.abort();
            self.heartbeat_task = None;
        }
    }

    fn handle_input(&mut self, key: &crossterm::event::KeyEvent) -> PageResponse {
        use crossterm::event::KeyCode;
        match key.code {
            KeyCode::Esc => { if let Some(h) = &self.current_handle { h.clear(); } PageResponse::None }
            KeyCode::Char('q') => {
                if let Some(tx) = self.stop_tx.take() { let _ = tx.send(true); } // true = Shutdown
                PageResponse::SwitchTo(Box::new(super::workflow_list::WorkflowList::new(self.api_client.clone(), self.event_tx.clone())))
            }
            KeyCode::Char('p') => {
                self.toggle_pause();
                PageResponse::None
            }
            KeyCode::Up | KeyCode::Char('k') => {
                if self.scroll_offset > 0 { self.scroll_offset -= 1; }
                self.auto_scroll = false;
                PageResponse::None
            }
            KeyCode::Down | KeyCode::Char('j') => {
                self.scroll_offset += 1;
                // Simple heuristic: if we manually move down, we might want to re-enable auto-scroll
                // We'll let the draw() function clamp and potentially re-enable it
                PageResponse::None
            }
            _ => PageResponse::None,
        }
    }

    fn handle_tick(&mut self) -> PageResponse {
        let h = match &self.current_handle { Some(h) => h, None => return PageResponse::None };
        let mags = h.analyzer_rx.borrow().clone();
        
        // 1. Sliding Window Energy (Idle detection)
        let raw_max = mags.iter().cloned().fold(0.0, f32::max);
        self.energy_window.push_back(raw_max);
        if self.energy_window.len() > 8 { self.energy_window.pop_front(); }
        let avg_energy = self.energy_window.iter().sum::<f32>() / self.energy_window.len() as f32;
        self.agent_playback_active = avg_energy > 0.02;

        // 2. Professional Gravity/Logarithmic Visualization Analysis
        let count = self.visualizer_data.len();
        for i in 0..count {
            let idx = (i * mags.len() / (count * 2)) % mags.len();
            let raw_mag = mags.get(idx).cloned().unwrap_or(0.0);
            
            // Logarithmic Scaling (approximate decibel feel)
            let log_mag = (raw_mag * 10.0).log10().clamp(-2.0, 0.5); 
            let normalized_target = (log_mag + 2.0) / 2.5; // Map [-2, 0.5] to [0, 1]

            // Gravity/Smoothing: Fast rise, slow fall
            if normalized_target > self.visualizer_data[i] {
                self.visualizer_data[i] = normalized_target;
            } else {
                self.visualizer_data[i] = (self.visualizer_data[i] - 0.15).max(0.0); // Constant decay rate
            }
        }
        PageResponse::None
    }

    fn handle_session_update(&mut self, se: &SessionEvent) -> PageResponse {
        match se { 
            SessionEvent::Connected(h) => self.current_handle = Some(h.clone()), 
            SessionEvent::Message(msg) => {
                if msg.p_type == PacketType::SessionId {
                    let id = String::from_utf8_lossy(&msg.payload).to_string();
                    info!("UI: Persistent Session ID captured: {}", id);
                    self.session_id = Some(id);
                } else {
                    self.process_protocol_message(msg);
                }
            } 
            SessionEvent::SpeakerState { role, speaking } => {
                if role == "user" { self.user_speaking = *speaking; }
                else { self.agent_speaking = *speaking; }
            }
            SessionEvent::Error(err) => {
                error!("Session Error: {}", err);
                self.error_message = Some(err.clone());
            } 
        }
        PageResponse::None
    }

    fn process_protocol_message(&mut self, p: &Packet) {
        if p.p_type != PacketType::Caption && p.p_type != PacketType::Text { return; }
        let bytes = &p.payload;
        let sub: serde_json::Value = match serde_json::from_slice(bytes) { Ok(v) => v, Err(_) => return };
        let role = sub["role"].as_str().unwrap_or("assistant").to_string();
        let text = sub["text"].as_str().unwrap_or("").to_string();
        let is_final = sub["is_final"].as_bool().unwrap_or(true);
        self.push_subtitle(role, text, is_final);
    }

    fn push_subtitle(&mut self, role: String, text: String, is_final: bool) {
        // 0. Normalize text to avoid common markdown parsing glitches (like \r\n or trailing whitespace)
        let normalized_text = text.replace('\r', "").trim_end().to_string();
        if normalized_text.is_empty() && !is_final {
            return;
        }

        // 1. Markdown 解析
        let rendered = tui_markdown::from_str(&normalized_text);
        let mut cached_lines: Vec<Line<'static>> = rendered.lines.into_iter().map(|line| {
            let spans: Vec<Span<'static>> = line.spans.into_iter().map(|s| {
                let mut span = Span::from(s.content.into_owned());
                span.style = convert_style(s.style);
                span
            }).collect();
            Line::from(spans)
        }).collect();

        // 2. 注入 Role 前缀 (保留配色)
        if !cached_lines.is_empty() {
            let color = if role == "user" { Color::Green } else { Color::Magenta };
            let prefix = Span::styled(format!("{}: ", role), Style::default().fg(color).add_modifier(Modifier::BOLD));
            cached_lines[0].spans.insert(0, prefix);
        }

        // 3. 更新或推入
        if let Some(last) = self.subtitles.back_mut() {
            if last.role == role && !last.is_final {
                last.text = normalized_text;
                last.is_final = is_final;
                last.cached_lines = cached_lines;
                return;
            }
        }
        
        if self.subtitles.len() >= 50 { self.subtitles.pop_front(); }
        self.subtitles.push_back(Subtitle { role, text: normalized_text, is_final, cached_lines });
    }
}

impl Page for Chat {
    fn handle_event(&mut self, event: &Event) -> PageResponse {
        self.init_session();
        match event { 
            Event::Input(key) => self.handle_input(key), 
            Event::Tick => self.handle_tick(), 
            Event::SessionUpdate(se) => self.handle_session_update(se), 
            _ => PageResponse::None 
        }
    }

    fn draw(&mut self, f: &mut Frame) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(3), // Header
                Constraint::Length(10), // Waveform
                Constraint::Min(10),   // Subtitles
                Constraint::Length(3), // Footer
            ])
            .split(f.area());
        
        let display_agent_speaking = self.agent_speaking || self.agent_playback_active;

        // 1. Header
        let user_status = if self.user_speaking { "● SPEAKING" } else { "○ IDLE" };
        let agent_status = if display_agent_speaking { "● SPEAKING" } else { "○ IDLE" };
        
        let header_style = if self.error_message.is_some() {
            Style::default().fg(Color::Red)
        } else if self.user_speaking {
            Style::default().fg(Color::Green)
        } else if display_agent_speaking {
            Style::default().fg(Color::Magenta)
        } else {
            Style::default().fg(Color::White)
        };

        let header_text = if let Some(err) = &self.error_message {
            format!(" Voce Live | ERROR: {} ", err)
        } else if self.is_paused {
            format!(" Voce Live | PAUSED (Session: {}***) ", self.session_id.as_ref().map(|s| &s[..6]).unwrap_or("Unknown"))
        } else {
            format!(" Voce Live | User: {} | Agent: {} ", user_status, agent_status)
        };

        f.render_widget(Paragraph::new(header_text)
            .block(Block::default().borders(Borders::ALL))
            .style(header_style), 
            chunks[0]
        );

        // 2. Pro Waveform (Liquid Animation)
        let wf_block = Block::default().title(" Spectrum Matrix ").borders(Borders::ALL);
        let wf_inner = wf_block.inner(chunks[1]);
        f.render_widget(wf_block, chunks[1]);
        
        let wf_color = if self.user_speaking { Color::Green } else if display_agent_speaking { Color::Magenta } else { Color::DarkGray };
        self.draw_pro_visualizer(f, wf_inner, wf_color);

        // 3. Middle Area (Split Subtitles and Monitoring)
        let mid_chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([
                Constraint::Percentage(70), // Subtitles
                Constraint::Percentage(30), // Stats Dashboard
            ])
            .split(chunks[2]);

        // --- Subtitles Rendering (Markdown Display & Dynamic Wrapping) ---
        let area_width = mid_chunks[0].width.saturating_sub(2).max(1) as usize;
        let area_height = mid_chunks[0].height.saturating_sub(2).max(1);
        
        let mut physical_lines = Vec::new();
        for s in &self.subtitles {
            for line in &s.cached_lines {
                physical_lines.extend(self.wrap_line(line.clone(), area_width));
            }
        }

        let total_lines = physical_lines.len() as u16;
        if self.auto_scroll {
            self.scroll_offset = total_lines.saturating_sub(area_height);
        } else {
            let max_scroll = total_lines.saturating_sub(area_height);
            if self.scroll_offset >= max_scroll {
                self.auto_scroll = true;
                self.scroll_offset = max_scroll;
            }
        }

        f.render_widget(Paragraph::new(physical_lines)
            .block(Block::default().title(" Transcription ").borders(Borders::ALL))
            .scroll((self.scroll_offset, 0)), mid_chunks[0]);

        // --- Stats Dashboard Rendering (Pure Numerical) ---
        let mut stats_text = vec![Line::from(vec![Span::styled(" LOADING MONITOR...", Style::default().fg(Color::DarkGray))])];

        if let Ok(data) = MONITOR_DATA.try_read() {
            if let Some(last) = &data.last_raw {
                stats_text = vec![
                    Line::from(vec![Span::styled(" ● ", Style::default().fg(Color::Green)), Span::styled("Active Sessions", Style::default().fg(Color::Gray))]),
                    Line::from(vec![Span::raw("   Count: "), Span::styled(last.active_sessions.to_string(), Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD))]),
                    
                    Line::from(vec![Span::raw(" ")]), // Spacer
                    
                    Line::from(vec![Span::styled(" ● ", Style::default().fg(Color::Blue)), Span::styled("Connectivity", Style::default().fg(Color::Gray))]),
                    Line::from(vec![Span::raw("   Socket: "), Span::styled(last.active_connections.to_string(), Style::default().fg(Color::White))]),
                    Line::from(vec![Span::raw("   Audio: "), Span::styled(last.active_audio_count.to_string(), Style::default().fg(Color::Magenta))]),
                    
                    Line::from(vec![Span::raw(" ")]), // Spacer
                    
                    Line::from(vec![Span::styled(" ● ", Style::default().fg(Color::Yellow)), Span::styled("System Health", Style::default().fg(Color::Gray))]),
                    Line::from(vec![Span::raw("   Memory: "), Span::styled(format!("{}MB", last.heap_inuse / 1024 / 1024), Style::default().fg(Color::White))]),
                    Line::from(vec![Span::raw("   GC Cnt: "), Span::styled(last.num_gc.to_string(), Style::default().fg(Color::DarkGray))]),
                ];
            }
        }

        f.render_widget(Paragraph::new(stats_text)
            .block(Block::default().title(" Monitor ").borders(Borders::ALL))
            .wrap(Wrap { trim: true }), mid_chunks[1]);

        // 4. Footer
        let footer_text = if self.is_paused {
            " [p] Resume Session | [q] Quit to List "
        } else {
            " [p] Pause | [ESC] Interrupt Signal | [q] Return to List "
        };
        f.render_widget(Paragraph::new(footer_text).block(Block::default().borders(Borders::ALL)), chunks[3]);
    }
}

impl Chat {
    fn draw_pro_visualizer(&self, f: &mut Frame, area: Rect, color: Color) {
        if area.width == 0 || area.height == 0 { return; }
        let bar_width = area.width as usize;
        let bar_height = area.height as usize;
        
        for row in 0..bar_height {
            let mut spans = Vec::with_capacity(bar_width);
            let threshold = (bar_height - row) as f32 / bar_height as f32;
            let step = 1.0 / bar_height as f32;

            for col in 0..bar_width {
                let data_idx = (col * self.visualizer_data.len() / bar_width) % self.visualizer_data.len();
                let val = self.visualizer_data[data_idx];
                
                let symbol = if val >= threshold {
                    "█"
                } else if val >= threshold - (step * 0.5) {
                    "▄"
                } else {
                    " "
                };
                spans.push(Span::styled(symbol, Style::default().fg(color)));
            }
            f.render_widget(Paragraph::new(Line::from(spans)), Rect::new(area.x, area.y + row as u16, area.width, 1));
        }
    }

    /// Helper to wrap a Line (potentially with multiple Spans) to a specific width.
    fn wrap_line<'a>(&self, line: Line<'a>, width: usize) -> Vec<Line<'a>> {
        let mut result = Vec::new();
        let mut current_spans = Vec::<Span<'a>>::new();
        let mut current_width = 0;

        for span in line.spans {
            let style = span.style;
            for c in span.content.chars() {
                let char_str = c.to_string();
                let cw = UnicodeWidthStr::width(char_str.as_str());

                // Wrap if adding this char exceeds width
                if current_width + cw > width && !current_spans.is_empty() {
                    result.push(Line::from(std::mem::take(&mut current_spans)));
                    current_width = 0;
                }

                // Append to existing span if style matches, otherwise push new one
                if let Some(last) = current_spans.last_mut().filter(|s| s.style == style) {
                    last.content.to_mut().push(c);
                } else {
                    current_spans.push(Span::styled(char_str, style));
                }

                current_width += cw;
            }
        }

        if !current_spans.is_empty() {
            result.push(Line::from(current_spans));
        }

        if result.is_empty() {
            result.push(Line::from(""));
        }
        result
    }
}

/// Helper to convert ratatui-core Style to ratatui Style.
fn convert_style(s: ratatui_core::style::Style) -> Style {
    let mut style = Style::default()
        .add_modifier(convert_modifier(s.add_modifier))
        .remove_modifier(convert_modifier(s.sub_modifier));
    
    if let Some(fg) = s.fg {
        style = style.fg(convert_color(fg));
    }
    if let Some(bg) = s.bg {
        style = style.bg(convert_color(bg));
    }
    style
}

/// Helper to convert ratatui-core Color to ratatui Color.
fn convert_color(c: ratatui_core::style::Color) -> Color {
    use ratatui_core::style::Color::*;
    match c {
        Reset => Color::Reset,
        Black => Color::Black,
        Red => Color::Red,
        Green => Color::Green,
        Yellow => Color::Yellow,
        Blue => Color::Blue,
        Magenta => Color::Magenta,
        Cyan => Color::Cyan,
        Gray => Color::Gray,
        DarkGray => Color::DarkGray,
        LightRed => Color::LightRed,
        LightGreen => Color::LightGreen,
        LightYellow => Color::LightYellow,
        LightBlue => Color::LightBlue,
        LightMagenta => Color::LightMagenta,
        LightCyan => Color::LightCyan,
        White => Color::White,
        Rgb(r, g, b) => Color::Rgb(r, g, b),
        Indexed(i) => Color::Indexed(i),
    }
}

/// Helper to convert ratatui-core Modifier to ratatui Modifier.
fn convert_modifier(m: ratatui_core::style::Modifier) -> Modifier {
    Modifier::from_bits_truncate(m.bits())
}
