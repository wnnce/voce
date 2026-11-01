// Worklets live in public/ so they are served as standalone modules: bundlers
// would otherwise inline or transform them, and addModule needs an unprocessed
// script at a stable URL (data: URLs also trip strict CSP).
const base = import.meta.env.BASE_URL;
const captureWorkletUrl = `${base}worklets/capture-processor.js`;
const playbackWorkletUrl = `${base}worklets/playback-processor.js`;

/**
 * One AudioContext for the whole page.
 *
 * Capture and playback must share a clock for the browser's AEC to line up its
 * delayed reference, and Chromium caps the number of hardware contexts. The
 * context runs at the device rate — resampling is explicit inside the worklets.
 */
let context: AudioContext | null = null;
let modulesReady: Promise<void> | null = null;

export async function ensureAudioContext(): Promise<AudioContext> {
  if (!context) {
    context = new AudioContext({ latencyHint: 'interactive' });
  }
  if (context.state === 'suspended') {
    await context.resume();
  }
  if (!modulesReady) {
    modulesReady = (async () => {
      const ctx = context as AudioContext;
      await ctx.audioWorklet.addModule(captureWorkletUrl);
      await ctx.audioWorklet.addModule(playbackWorkletUrl);
    })();
  }
  await modulesReady;
  return context;
}

export function currentAudioContext(): AudioContext | null {
  return context;
}

export function deviceSampleRate(): number {
  return context?.sampleRate ?? 48000;
}

export interface CaptureHandle {
  stop: () => void;
}

async function applySink(element: HTMLAudioElement, deviceId: string): Promise<boolean> {
  if (!deviceId) return false;
  const withSink = element as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
  if (!withSink.setSinkId) return false;
  try {
    await withSink.setSinkId(deviceId);
    return true;
  } catch {
    return false;
  }
}

export interface PlaybackHandle {
  /** Feeds a PCM16 frame; length is arbitrary. */
  push: (payload: Uint8Array) => void;
  /**
   * Drops everything queued, immediately. Called on 0x11 Interrupter, on Close
   * and on teardown — messages are ordered, so a clear posted after a burst of
   * writes discards exactly that burst.
   */
  clear: () => void;
  /** Routes the element to an explicit output device. */
  setSink: (deviceId: string) => Promise<boolean>;
  close: () => void;
}

export async function startCapture(
  ctx: AudioContext,
  stream: MediaStream,
  onFrame: (frame: ArrayBuffer) => void,
  onLevel: (rms: number) => void,
): Promise<CaptureHandle> {
  const source = ctx.createMediaStreamSource(stream);
  const node = new AudioWorkletNode(ctx, 'voce-capture', {
    numberOfInputs: 1,
    numberOfOutputs: 1,
    processorOptions: { targetRate: 16000, frameMs: 20 },
  });

  node.port.onmessage = (event: MessageEvent<{ type: string; data: ArrayBuffer; level?: number }>) => {
    const message = event.data;
    if (message.type !== 'frame') return;
    onFrame(message.data);
    if (typeof message.level === 'number') onLevel(message.level);
  };

  // A node that never reaches the destination may not be pulled at all; route
  // through a silent gain instead of dropping the graph.
  const sink = ctx.createGain();
  sink.gain.value = 0;
  source.connect(node);
  node.connect(sink);
  sink.connect(ctx.destination);

  return {
    stop: () => {
      node.port.onmessage = null;
      source.disconnect();
      node.disconnect();
      sink.disconnect();
    },
  };
}

/**
 * Playback goes through a real <audio> element rather than straight to
 * `ctx.destination`.
 *
 * Chromium feeds the media element's output into the echo-cancellation
 * reference chain; a bare AudioWorklet graph is not guaranteed to be part of
 * it. The worklet still does the resampling and buffering — it just hands the
 * result to a MediaStream the element renders.
 */
export function startPlayback(ctx: AudioContext): PlaybackHandle {
  const node = new AudioWorkletNode(ctx, 'voce-playback', {
    numberOfInputs: 0,
    numberOfOutputs: 1,
    outputChannelCount: [1],
  });

  const sink = ctx.createMediaStreamDestination();
  node.connect(sink);

  const element = document.createElement('audio');
  element.autoplay = true;
  element.hidden = true;
  element.setAttribute('playsinline', '');
  element.srcObject = sink.stream;
  document.body.appendChild(element);
  // A rejected play() only means autoplay was blocked; the stream starts on
  // the next user gesture.
  void element.play().catch(() => undefined);

  return {
    push: (payload: Uint8Array) => {
      if (payload.byteLength < 2) return;
      const samples = new Int16Array(payload.byteLength >> 1);
      new Uint8Array(samples.buffer).set(payload.subarray(0, samples.byteLength));
      node.port.postMessage({ type: 'write', data: samples.buffer }, [samples.buffer]);
    },
    clear: () => node.port.postMessage({ type: 'clear' }),
    setSink: (deviceId) => applySink(element, deviceId),
    close: () => {
      element.pause();
      element.srcObject = null;
      element.remove();
      node.disconnect();
      sink.disconnect();
    },
  };
}
