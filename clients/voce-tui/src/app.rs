use std::sync::Arc;
use tokio::sync::mpsc;
use crate::api::{ApiClient, WorkflowConfig};
use crate::pages::{Page, workflow_list::WorkflowList};
use crate::common::events::Event;
use tracing::info;

#[derive(PartialEq, Eq, Debug, Clone, Copy)]
pub enum AppState {
    Running,
    Finished,
}

pub struct App {
    pub current_page: Box<dyn Page>,
    pub state: AppState,
    pub cached_workflows: Option<Vec<WorkflowConfig>>,
}

impl App {
    pub fn new(api_client: Arc<ApiClient>, event_tx: mpsc::Sender<Event>) -> Self {
        info!("Initializing App with provided ApiClient.");
        Self { // Added Self { to fix syntax
            current_page: Box::new(WorkflowList::new(api_client, event_tx)),
            state: AppState::Running,
            cached_workflows: None,
        }
    }

    pub async fn handle_event(&mut self, event: &Event) {
        // 1. Cache the workflow list if it arrives
        if let Event::WorkflowsLoaded(w) = event {
            self.cached_workflows = Some(w.clone());
        }

        // 2. Delegate event to current page
        match self.current_page.handle_event(event) {
            crate::pages::PageResponse::SwitchTo(mut new_page) => {
                info!("Navigator: Switching Page...");
                
                // 3. Sync cached data to the new page immediately
                if let Some(w) = &self.cached_workflows {
                    new_page.handle_event(&Event::WorkflowsLoaded(w.clone()));
                }
                
                self.current_page = new_page;
            }
            crate::pages::PageResponse::Exit => {
                info!("Navigator: Received Exit Signal.");
                self.state = AppState::Finished;
            }
            _ => {}
        }
    }

    pub fn draw(&mut self, f: &mut ratatui::Frame) {
        self.current_page.draw(f);
    }
}
