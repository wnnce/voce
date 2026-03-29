use ratatui::Frame;
use crate::common::events::Event;

pub enum PageResponse {
    None,
    SwitchTo(Box<dyn Page>),
    Exit,
}

pub trait Page: Send + Sync {
    fn handle_event(&mut self, event: &Event) -> PageResponse;
    fn draw(&mut self, f: &mut Frame);
}

pub mod workflow_list;
pub mod property_editor;
pub mod chat;
pub mod monitor_dashboard;
