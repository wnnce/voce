mod api;
mod app;
mod audio;
mod common;
mod config;
mod pages;
mod protocol;
mod session;
mod monitor;
mod ui;

use anyhow::Result;
use app::{App, AppState};
use std::sync::Arc;
use tokio::sync::mpsc;
use tracing::{info, error};
use crate::api::ApiClient;
use crate::config::Config;

#[tokio::main]
async fn main() -> Result<()> {
    // 1. Initialize Logging
    let file_appender = tracing_appender::rolling::never(".", "voce-tui.log");
    let (non_blocking, _guard) = tracing_appender::non_blocking(file_appender);
    tracing_subscriber::fmt()
        .with_writer(non_blocking)
        .with_env_filter("info,voce_tui=debug")
        .init();

    info!("Starting Voce-TUI application...");

    // 2. Load Configuration
    let config = Config::load();
    let api_client = Arc::new(ApiClient::new(&config.api_url));

    // 3. Start Terminal UI & Monitor
    let (event_tx, mut event_rx) = mpsc::channel(100);
    
    // Initialize Global Monitor Worker
    let monitor_worker = monitor::MonitorWorker::new(api_client.clone());
    tokio::spawn(async move {
        monitor_worker.start().await;
    });

    let mut app = App::new(api_client.clone(), event_tx.clone());

    let mut terminal = ui::init_terminal()?;
    app.state = AppState::Running;

    // 4. Initial Data Fetch (Moved to background or here)
    let startup_api = api_client.clone();
    let startup_event_tx = event_tx.clone();
    tokio::spawn(async move {
        match startup_api.list_workflows().await {
            Ok(w) => {
                info!("Initial workflows loaded: {} items", w.len());
                let _ = startup_event_tx.send(common::events::Event::WorkflowsLoaded(w)).await;
            }
            Err(e) => error!("Failed to load initial workflows: {}", e),
        }
    });

    // 5. Tasks Setup
    let ticker_tx = event_tx.clone();
    let _ticker = tokio::spawn(async move {
        let mut interval = tokio::time::interval(std::time::Duration::from_millis(50));
        while ticker_tx.send(common::events::Event::Tick).await.is_ok() {
            interval.tick().await;
        }
    });

    let input_tx = event_tx.clone();
    let _input_handler = tokio::spawn(async move {
        loop {
            if let Some(event) = ui::next_event().await {
                if input_tx.send(event).await.is_err() { break; }
            }
            // Small sleep to prevent tight loop if poll errors
            tokio::time::sleep(std::time::Duration::from_millis(10)).await;
        }
    });

    // 6. Main Render Loop
    while app.state == AppState::Running {
        terminal.draw(|f| app.draw(f))?;
        if let Some(event) = event_rx.recv().await {
            app.handle_event(&event).await;
        }
    }

    // 7. Cleanup
    ui::restore_terminal()?;
    info!("Voce-TUI terminated gracefully.");
    Ok(())
}
