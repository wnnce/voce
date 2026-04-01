use std::sync::Arc;
use tokio::sync::mpsc;
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use crate::common::events::Event;
use crate::pages::{Page, PageResponse};
use super::property_editor::PropertyEditor; // Corrected relative import
use crate::api::{WorkflowConfig, ApiClient};

pub struct WorkflowList {
    api_client: Arc<ApiClient>,
    event_tx: mpsc::Sender<Event>,
    workflows: Vec<WorkflowConfig>,
    list_state: ListState,
}

impl WorkflowList {
    pub fn new(api_client: Arc<ApiClient>, event_tx: mpsc::Sender<Event>) -> Self {
        Self { api_client, event_tx, workflows: Vec::new(), list_state: ListState::default() }
    }

    fn handle_input(&mut self, key: &crossterm::event::KeyEvent) -> PageResponse {
        use crossterm::event::KeyCode;
        match key.code {
            KeyCode::Up => self.move_selection(-1),
            KeyCode::Down => self.move_selection(1),
            KeyCode::Enter => self.confirm_selection(),
            KeyCode::Char('m') => PageResponse::SwitchTo(Box::new(super::monitor_dashboard::MonitorDashboard::new(self.api_client.clone(), self.event_tx.clone()))),
            KeyCode::Char('q') | KeyCode::Esc => PageResponse::Exit,
            _ => PageResponse::None,
        }
    }

    fn move_selection(&mut self, delta: i32) -> PageResponse {
        if self.workflows.is_empty() { return PageResponse::None; }
        let current = self.list_state.selected().unwrap_or(0);
        let len = self.workflows.len() as i32;
        let new_idx = (current as i32 + delta).rem_euclid(len) as usize;
        self.list_state.select(Some(new_idx));
        PageResponse::None
    }

    fn confirm_selection(&mut self) -> PageResponse {
        let idx = match self.list_state.selected() {
            Some(i) => i,
            None => return PageResponse::None,
        };
        let workflow = self.workflows[idx].clone();
        PageResponse::SwitchTo(Box::new(PropertyEditor::new(
            workflow, 
            self.api_client.clone(), 
            self.event_tx.clone()
        )))
    }
}

impl Page for WorkflowList {
    fn handle_event(&mut self, event: &Event) -> PageResponse {
        match event {
            Event::Input(key) => self.handle_input(key),
            Event::WorkflowsLoaded(w) => {
                self.workflows = w.clone();
                if !self.workflows.is_empty() { self.list_state.select(Some(0)); }
                PageResponse::None
            }
            _ => PageResponse::None,
        }
    }

    fn draw(&mut self, f: &mut Frame) {
        let area = centered_rect(60, 40, f.area());
        let items: Vec<ListItem> = self.workflows.iter().map(|w| ListItem::new(format!(" {} ", w.name))).collect();
        let list = List::new(items)
            .block(Block::default().title(" 1. Select Workflow ").borders(Borders::ALL).border_style(Style::default().fg(Color::Cyan)))
            .highlight_style(Style::default().bg(Color::Cyan).fg(Color::Black).add_modifier(Modifier::BOLD))
            .highlight_symbol(">> ");
        f.render_stateful_widget(list, area, &mut self.list_state);

        // Help text
        let help = Paragraph::new(" [m] Open Global Monitor | [ENTER] Select | [q] Quit ")
            .style(Style::default().fg(Color::DarkGray));
        let mut help_area = area;
        help_area.y += area.height;
        help_area.height = 1;
        f.render_widget(help, help_area);
    }
}

fn centered_rect(percent_x: u16, percent_y: u16, r: Rect) -> Rect {
    let popup_layout = Layout::default().direction(Direction::Vertical).constraints([Constraint::Percentage((100 - percent_y) / 2), Constraint::Percentage(percent_y), Constraint::Percentage((100 - percent_y) / 2)]).split(r);
    Layout::default().direction(Direction::Horizontal).constraints([Constraint::Percentage((100 - percent_x) / 2), Constraint::Percentage(percent_x), Constraint::Percentage((100 - percent_x) / 2)]).split(popup_layout[1])[1]
}
