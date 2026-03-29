use anyhow::{Result, anyhow};
use futures_util::{SinkExt, StreamExt};
use std::sync::Arc;
use tokio::sync::{mpsc, oneshot};
use std::sync::atomic::Ordering;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::protocol::Message as WsMessage;
use crate::api::ApiClient;
use crate::audio::{AudioEngine, AudioHandle};
use crate::protocol::{Packet, PacketType};
use ringbuf::traits::Producer;
use tracing::{info, error};

pub enum SessionEvent {
    Connected(Arc<AudioHandle>),
    Message(Packet),
    SpeakerState { role: String, speaking: bool },
    Error(String),
}

pub struct SessionManager {
    api_client: Arc<ApiClient>,
    event_tx: mpsc::Sender<SessionEvent>,
}

struct TaskGuard(Vec<tokio::task::JoinHandle<()>>);

impl Drop for TaskGuard {
    fn drop(&mut self) {
        for t in &self.0 {
            t.abort();
        }
    }
}

impl SessionManager {
    pub fn new(api_client: Arc<ApiClient>, event_tx: mpsc::Sender<SessionEvent>) -> Self {
        Self { api_client, event_tx }
    }

    pub async fn start(self, workflow: String, properties: serde_json::Value, session_id: Option<String>, stop_rx: oneshot::Receiver<bool>) {
        info!("Starting Session Manager for workflow: {} with properties: {}", workflow, properties);
        if let Err(e) = self.run(workflow, properties, session_id, stop_rx).await {
            error!("Session critical failure: {}", e);
            let _ = self.event_tx.send(SessionEvent::Error(e.to_string())).await;
        }
    }

    async fn run(&self, workflow: String, properties: serde_json::Value, session_id: Option<String>, mut stop_rx: oneshot::Receiver<bool>) -> Result<()> {
        let ticket = match session_id {
            Some(id) => id,
            None => self.api_client.create_ticket(&workflow, properties).await?
        };
        
        let _ = self.event_tx.send(SessionEvent::Message(Packet::new(PacketType::SessionId, ticket.as_bytes().to_vec()))).await;

        let ws_base_url = self.api_client.get_base_url().replace("http://", "ws://").replace("https://", "wss://");
        let ws_url = format!("{}/realtime/{}", ws_base_url, ticket);
        
        let (ws_stream, _) = connect_async(ws_url).await.map_err(|e| anyhow!("WS Connect Error: {}", e))?;
        let (mut ws_tx, mut ws_rx) = ws_stream.split();
        
        let (ws_send_tx, mut ws_send_rx) = mpsc::channel::<WsMessage>(100);
        let mut tasks = TaskGuard(Vec::new());

        // Task: WS Dedicated Sender
        tasks.0.push(tokio::spawn(async move {
            while let Some(msg) = ws_send_rx.recv().await {
                if ws_tx.send(msg).await.is_err() { break; }
            }
            let _ = ws_tx.close().await;
        }));

        // Task: Heartbeat (5s)
        let hb_ws_tx = ws_send_tx.clone();
        tasks.0.push(tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(5));
            loop {
                interval.tick().await;
                if hb_ws_tx.send(WsMessage::Ping(vec![])).await.is_err() {
                    break;
                }
            }
        }));

        let engine = AudioEngine::new()?;
        let (handle, mut mic_rx, _out, _in) = engine.start()?; 
        let handle_arc = Arc::new(handle);
        let _ = self.event_tx.send(SessionEvent::Connected(handle_arc.clone())).await;

        // Task: Outbound Audio
        let ui_ws_tx = ws_send_tx.clone();
        tasks.0.push(tokio::spawn(async move {
            let mut buf = Vec::with_capacity(800);
            while let Some(data) = mic_rx.recv().await {
                buf.extend(data);
                if buf.len() < 1600 { continue; }
                let msg = Packet::new(PacketType::Audio, buf.drain(..1600).collect());
                if ui_ws_tx.send(WsMessage::Binary(msg.marshal())).await.is_err() { break; }
            }
        }));

        // Task: Inbound Pacing
        let (playback_in_tx, mut playback_in_rx) = mpsc::channel::<Vec<f32>>(1000);
        let h_inner = handle_arc.clone();
        tasks.0.push(tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_millis(20));
            let mut buffer = std::collections::VecDeque::new();
            let chunk_size = (h_inner.sample_rate / 50) as usize; 
            let mut last_clear_seq = 0;

            loop {
                interval.tick().await;
                let current_seq = h_inner.clear_signal.load(Ordering::Relaxed);
                if current_seq > last_clear_seq {
                    buffer.clear();
                    while playback_in_rx.try_recv().is_ok() {}
                    last_clear_seq = current_seq;
                }
                while let Ok(samples) = playback_in_rx.try_recv() { buffer.extend(samples); }
                
                if buffer.len() < chunk_size { continue; }
                let chunk: Vec<f32> = buffer.drain(..chunk_size).collect();
                if let Ok(mut tx) = h_inner.playback_tx.lock() { let _ = tx.push_slice(&chunk); }
            }
        }));

        // Main Loop
        loop {
            tokio::select! {
                res = ws_rx.next() => {
                    let msg = match res { Some(Ok(m)) => m, _ => break };
                    if self.process_ws_message(msg, &handle_arc, &playback_in_tx).await.is_err() { break; }
                }
                shutdown_res = &mut stop_rx => {
                    let should_shutdown = shutdown_res.unwrap_or(true);
                    if should_shutdown {
                        let close_msg = Packet::new(PacketType::Close, vec![]);
                        let _ = ws_send_tx.send(WsMessage::Binary(close_msg.marshal())).await;
                    }
                    break;
                }
            }
        }
        Ok(())
    }

    async fn process_ws_message(
        &self, 
        ws_msg: WsMessage, 
        handle: &Arc<AudioHandle>, 
        playback_in_tx: &mpsc::Sender<Vec<f32>>
    ) -> Result<()> {
        let bytes = match ws_msg { WsMessage::Binary(b) => b, _ => return Ok(()) };
        let p = Packet::unmarshal(&bytes).map_err(|e| anyhow!("Protocol Error: {}", e))?;
        
        match p.p_type {
            PacketType::Audio => {
                self.handle_audio_payload(&p.payload, handle, playback_in_tx).await?;
            }
            PacketType::Interrupter => {
                handle.clear();
                let _ = self.event_tx.send(SessionEvent::Message(p)).await;
            }
            PacketType::AgentSpeechStart => {
                let _ = self.event_tx.send(SessionEvent::SpeakerState { role: "agent".into(), speaking: true }).await;
            }
            PacketType::AgentSpeechEnd => {
                let _ = self.event_tx.send(SessionEvent::SpeakerState { role: "agent".into(), speaking: false }).await;
            }
            PacketType::UserSpeechStart => {
                let _ = self.event_tx.send(SessionEvent::SpeakerState { role: "user".into(), speaking: true }).await;
            }
            PacketType::UserSpeechEnd => {
                let _ = self.event_tx.send(SessionEvent::SpeakerState { role: "user".into(), speaking: false }).await;
            }
            _ => { let _ = self.event_tx.send(SessionEvent::Message(p)).await; }
        }
        Ok(())
    }

    async fn handle_audio_payload(
        &self, 
        payload: &[u8], 
        handle: &Arc<AudioHandle>, 
        playback_in_tx: &mpsc::Sender<Vec<f32>>
    ) -> Result<()> {
        if payload.is_empty() { return Ok(()); }
        let samples: Vec<f32> = payload.chunks_exact(2)
            .map(|c| (i16::from_le_bytes([c[0], c[1]]) as f32) / 32768.1)
            .collect();
        let resampled = crate::audio::resample_linear(&samples, 16000, handle.sample_rate);
        let _ = playback_in_tx.send(resampled).await;
        Ok(())
    }
}
