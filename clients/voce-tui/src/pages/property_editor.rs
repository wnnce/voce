use std::sync::Arc;
use tokio::sync::mpsc;
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph, Wrap},
    Frame,
};
use crate::common::events::Event;
use crate::pages::{Page, PageResponse};
use super::chat::Chat; // Corrected relative import
use crate::api::{WorkflowConfig, ApiClient};

pub struct PropertyEditor {
    workflow: WorkflowConfig,
    api_client: Arc<ApiClient>,
    event_tx: mpsc::Sender<Event>,
    input: String,
    cursor_pos: usize,
}

impl PropertyEditor {
    pub fn new(workflow: WorkflowConfig, api_client: Arc<ApiClient>, event_tx: mpsc::Sender<Event>) -> Self {
        Self { api_client, event_tx, workflow, input: String::from("{}"), cursor_pos: 2 }
    }

    fn handle_input(&mut self, key: &crossterm::event::KeyEvent) -> PageResponse {
        use crossterm::event::KeyCode;
        match key.code {
            KeyCode::Esc => PageResponse::SwitchTo(Box::new(super::workflow_list::WorkflowList::new(self.api_client.clone(), self.event_tx.clone()))),
            KeyCode::Enter => self.start_session(),
            KeyCode::Char(c) => self.insert_char(c),
            KeyCode::Backspace => self.backspace(),
            _ => PageResponse::None,
        }
    }

    fn start_session(&mut self) -> PageResponse {
        let props: serde_json::Value = match serde_json::from_str(&self.input) {
            Ok(p) => p,
            Err(_) => return PageResponse::None,
        };
        PageResponse::SwitchTo(Box::new(Chat::new(
            self.workflow.name.clone(), props, self.api_client.clone(), self.event_tx.clone())))
    }

    fn insert_char(&mut self, c: char) -> PageResponse { self.input.insert(self.cursor_pos, c); self.cursor_pos += 1; PageResponse::None }
    fn backspace(&mut self) -> PageResponse { if self.cursor_pos == 0 { return PageResponse::None; } self.input.remove(self.cursor_pos - 1); self.cursor_pos -= 1; PageResponse::None }
}

impl Page for PropertyEditor {
    fn handle_event(&mut self, event: &Event) -> PageResponse {
        match event { Event::Input(key) => self.handle_input(key), _ => PageResponse::None }
    }

    fn draw(&mut self, f: &mut Frame) {
        let area = centered_rect(60, 30, f.area());
        let input = Paragraph::new(self.input.as_str())
            .block(Block::default().title(format!(" 2. Config for {} ", self.workflow.name)).borders(Borders::ALL).border_style(Style::default().fg(Color::Green)))
            .wrap(Wrap { trim: false });
        f.render_widget(input, area);
        
        let tips = Paragraph::new(vec![
            Line::from(vec![Span::raw("Press "), Span::styled("Enter", Style::default().fg(Color::Yellow)), Span::raw(" to Start Session")]),
            Line::from(vec![Span::raw("Press "), Span::styled("Esc", Style::default().fg(Color::Red)), Span::raw(" to go back")]),
        ]);
        let mut t_area = area; t_area.y += area.height; t_area.height = 2;
        f.render_widget(tips, t_area);
    }
}

fn centered_rect(percent_x: u16, percent_y: u16, r: Rect) -> Rect {
    let popup_layout = Layout::default().direction(Direction::Vertical).constraints([Constraint::Percentage((100 - percent_y) / 2), Constraint::Percentage(percent_y), Constraint::Percentage((100 - percent_y) / 2)]).split(r);
    Layout::default().direction(Direction::Horizontal).constraints([Constraint::Percentage((100 - percent_x) / 2), Constraint::Percentage(percent_x), Constraint::Percentage((100 - percent_x) / 2)]).split(popup_layout[1])[1]
}
