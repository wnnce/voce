use anyhow::{Result, anyhow};
use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use cpal::{Stream, StreamConfig};
use ringbuf::traits::{Consumer, Split};
use ringbuf::{HeapRb, HeapProd, HeapCons};
use rustfft::{FftPlanner, num_complex::Complex};
use std::collections::VecDeque;
use std::sync::{Arc, Mutex};
use std::sync::atomic::{AtomicU64, Ordering};
use tokio::sync::mpsc;
use tokio::sync::watch;
use webrtc_audio_processing::{Config, EchoCancellation, EchoCancellationSuppressionLevel, Processor, InitializationConfig};

pub struct SendStream(#[allow(dead_code)] pub Stream);
unsafe impl Send for SendStream {}
unsafe impl Sync for SendStream {}

pub struct AudioHandle {
    pub playback_tx: Arc<Mutex<HeapProd<f32>>>,
    pub analyzer_rx: watch::Receiver<Vec<f32>>,
    pub sample_rate: u32,
    pub clear_signal: Arc<AtomicU64>,
}

impl AudioHandle {
    pub fn clear(&self) {
        self.clear_signal.fetch_add(1, Ordering::SeqCst);
    }
}

pub struct AudioEngine {
    device: cpal::Device,
    config: StreamConfig,
}

impl AudioEngine {
    pub fn new() -> Result<Self> {
        let host = cpal::default_host();
        let device = host.default_output_device().ok_or_else(|| anyhow!("No Output Device"))?;
        let config = device.default_output_config().map_err(|e| anyhow!("Config Error: {}", e))?;
        Ok(Self { device, config: config.config() })
    }

    pub fn start(&self) -> Result<(AudioHandle, mpsc::Receiver<Vec<u8>>, SendStream, SendStream)> {
        let sample_rate = self.config.sample_rate.0;
        let clear_signal = Arc::new(AtomicU64::new(0));

        // 1. Setup Processor (Fixed at 48kHz which is the library default)
        let init_config = InitializationConfig {
            num_capture_channels: 1,
            num_render_channels: 1,
            ..InitializationConfig::default()
        };
        
        let mut ap = Processor::new(&init_config)?;
        let ap_config = Config {
            echo_cancellation: Some(EchoCancellation {
                suppression_level: EchoCancellationSuppressionLevel::High,
                enable_delay_agnostic: true,
                enable_extended_filter: true,
                stream_delay_ms: Some(10),
            }),
            ..Config::default()
        };
        ap.set_config(ap_config);
        let ap = Arc::new(Mutex::new(ap));

        // 2. Setup Pipelines
        let rb = HeapRb::<f32>::new(sample_rate as usize * 2);
        let (prod, cons) = rb.split();
        let (analyzer_tx, analyzer_rx) = watch::channel(vec![0.0; 256]);

        let out_stream = self.start_output_stream(cons, ap.clone(), analyzer_tx, clear_signal.clone())?;
        let (in_stream, mic_rx) = self.start_input_stream(ap.clone())?;

        Ok((AudioHandle {
            playback_tx: Arc::new(Mutex::new(prod)),
            analyzer_rx,
            sample_rate,
            clear_signal,
        }, mic_rx, SendStream(out_stream), SendStream(in_stream)))
    }

    fn start_output_stream(
        &self, 
        mut cons: HeapCons<f32>, 
        ap: Arc<Mutex<Processor>>, 
        analyzer_tx: watch::Sender<Vec<f32>>,
        clear_signal: Arc<AtomicU64>
    ) -> Result<Stream> {
        let sample_rate = self.config.sample_rate.0;
        let channels = self.config.channels as usize;
        let mut planner = FftPlanner::new();
        let fft = planner.plan_fft_forward(512);
        let fft_buf = Arc::new(Mutex::new(VecDeque::with_capacity(512)));
        let mut aec_accum = Vec::with_capacity(480);
        let mut last_clear_seq = 0;

        let stream = self.device.build_output_stream(
            &self.config,
            move |data: &mut [f32], _| {
                // Buffer management
                let current_seq = clear_signal.load(Ordering::Relaxed);
                if current_seq > last_clear_seq {
                    while cons.try_pop().is_some() {}
                    last_clear_seq = current_seq;
                }

                for chunk in data.chunks_exact_mut(channels) {
                    let s = cons.try_pop().unwrap_or(0.0);
                    for ch in chunk.iter_mut() { *ch = s; }
                }

                // Render AEC Frame (48kHz fixed)
                let mono: Vec<f32> = data.chunks(channels).map(|c| c.iter().sum::<f32>() / channels as f32).collect();
                for s in resample_linear(&mono, sample_rate, 48000) {
                    aec_accum.push(s);
                    if aec_accum.len() < 480 { continue; }
                    if let Ok(mut p) = ap.lock() { let _ = p.process_render_frame(&mut aec_accum); }
                    aec_accum.clear();
                }

                // Analyzer
                let mut f_buf = match fft_buf.lock() { Ok(b) => b, Err(_) => return };
                for &s in data.iter() {
                    if f_buf.len() >= 512 { f_buf.pop_front(); }
                    f_buf.push_back(s);
                }
                if f_buf.len() < 512 { return; }
                let mut input: Vec<Complex<f32>> = f_buf.iter().map(|&v| Complex::new(v, 0.0)).collect();
                fft.process(&mut input);
                let _ = analyzer_tx.send(input.iter().take(256).map(|c| c.norm()).collect());
            },
            |e| println!("Output Error: {}", e),
            None
        )?;
        stream.play()?;
        Ok(stream)
    }

    fn start_input_stream(&self, ap: Arc<Mutex<Processor>>) -> Result<(Stream, mpsc::Receiver<Vec<u8>>)> {
        let host = cpal::default_host();
        let device = host.default_input_device().ok_or_else(|| anyhow!("No Input Device"))?;
        let config = device.default_input_config()?.config();
        let (tx, rx) = mpsc::channel(100);
        
        let native_rate = config.sample_rate.0;
        let channels = config.channels as usize;
        let mut capture_accum = Vec::with_capacity(480);

        let stream = device.build_input_stream(
            &config,
            move |data: &[f32], _| {
                let mono: Vec<f32> = data.chunks(channels).map(|c| c.iter().sum::<f32>() / channels as f32).collect();
                for s in resample_linear(&mono, native_rate, 48000) {
                    capture_accum.push(s);
                    if capture_accum.len() < 480 { continue; }
                    
                    // WebRTC Capture
                    if let Ok(mut p) = ap.lock() { let _ = p.process_capture_frame(&mut capture_accum); }
                    
                    // Downsample to 16k and Send
                    let bytes: Vec<u8> = resample_linear(&capture_accum, 48000, 16000).iter()
                        .flat_map(|&s| ((s.clamp(-1.0, 1.0) * 32767.0) as i16).to_le_bytes())
                        .collect();
                    let _ = tx.try_send(bytes);
                    capture_accum.clear();
                }
            },
            |e| println!("Input Error: {}", e),
            None
        )?;
        stream.play()?;
        Ok((stream, rx))
    }
}

pub fn resample_linear(input: &[f32], from: u32, to: u32) -> Vec<f32> {
    if from == to { return input.to_vec(); }
    let ratio = to as f32 / from as f32;
    let out_len = (input.len() as f32 * ratio) as usize;
    let mut output = Vec::with_capacity(out_len);
    for i in 0..out_len {
        let pos = i as f32 / ratio;
        let idx = pos as usize;
        let frac = pos - idx as f32;
        if idx + 1 >= input.len() { output.push(input[idx]); continue; }
        output.push(input[idx] * (1.0 - frac) + input[idx + 1] * frac);
    }
    output
}
