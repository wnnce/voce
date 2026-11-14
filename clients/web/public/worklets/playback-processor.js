/**
 * Playback worklet: 16kHz mono PCM16 in, device-rate float out.
 *
 * A ring buffer consumer instead of one AudioBufferSourceNode per packet —
 * per-packet scheduling leaves audible seams at packet boundaries and makes
 * "drop everything right now" (server interrupt) impossible.
 *
 * The client holds audio, it does not throttle it: 60s of capacity absorbs
 * long pre-synthesised segments and network stalls. Only a hard overflow drops
 * the oldest samples, and 0x11 flushes everything.
 */
const CAPACITY_SECONDS = 60;

class PlaybackProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.sourceRate = 16000;
    this.ratio = this.sourceRate / sampleRate;

    this.capacity = Math.round(this.sourceRate * CAPACITY_SECONDS);
    this.ring = new Float32Array(this.capacity);
    this.read = 0;
    this.write = 0;
    this.available = 0;

    this.pos = 0;

    // Starting, resuming after a flush, or interrupting mid-waveform all step
    // the signal discontinuously, which is audible as a click. A 5ms ramp is
    // inaudible and removes it.
    this.rampSamples = Math.max(1, Math.round(sampleRate * 0.005));
    this.gain = 0;

    this.port.onmessage = (event) => {
      const message = event.data;
      if (message.type === 'write') {
        this.writeFrames(new Int16Array(message.data));
        return;
      }
      if (message.type === 'clear') {
        this.clear();
      }
    };
  }

  clear() {
    this.read = 0;
    this.write = 0;
    this.available = 0;
    this.pos = 0;
    this.gain = 0;
  }

  writeFrames(samples) {
    for (let i = 0; i < samples.length; i += 1) {
      if (this.available === this.capacity) {
        // Hard overflow only: hold as much as memory allows, then let the
        // newest audio win.
        this.read = (this.read + 1) % this.capacity;
        this.available -= 1;
      }
      this.ring[this.write] = samples[i] / 0x8000;
      this.write = (this.write + 1) % this.capacity;
      this.available += 1;
    }
  }

  nextSample() {
    if (this.available < 1) return 0;
    const a = this.ring[this.read];
    const b = this.available > 1 ? this.ring[(this.read + 1) % this.capacity] : a;
    return a + (b - a) * this.pos;
  }

  advance() {
    this.read = (this.read + 1) % this.capacity;
    this.available -= 1;
  }

  process(_inputs, outputs) {
    const channel = outputs[0] && outputs[0][0];
    if (!channel) return true;

    if (this.available < 1) {
      channel.fill(0);
      this.pos = 0;
      this.gain = 0;
      return true;
    }

    const step = 1 / this.rampSamples;
    for (let i = 0; i < channel.length; i += 1) {
      channel[i] = this.nextSample();
      if (this.gain < 1) {
        this.gain = Math.min(1, this.gain + step);
        channel[i] *= this.gain;
      }
      this.pos += this.ratio;
      while (this.pos >= 1) {
        this.pos -= 1;
        if (this.available > 0) this.advance();
      }
    }

    return true;
  }
}

registerProcessor('voce-playback', PlaybackProcessor);
