use crossterm::event::KeyEvent;
use crate::api::WorkflowConfig;
use crate::session::SessionEvent;

pub enum Event {
    Input(KeyEvent),
    Tick,
    SessionUpdate(SessionEvent),
    WorkflowsLoaded(Vec<WorkflowConfig>),
}
