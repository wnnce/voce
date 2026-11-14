/**
 * Capture worklet: microphone → 16kHz mono PCM16 frames.
 *
 * Runs on the audio thread. AEC/NS/AGC have already been applied upstream by
 * getUserMedia, so this stage only resamples and frames. Decimation uses a
 * windowed-sinc low-pass designed for the actual device ratio — plain sample
 * dropping would alias speech energy back below 8kHz and confuse the server VAD.
 */
class CaptureProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const opts = (options && options.processorOptions) || {};
    this.targetRate = opts.targetRate || 16000;
    this.frameMs = opts.frameMs || 20;

    this.frameSamples = Math.round((this.targetRate * this.frameMs) / 1000);
    this.ratio = sampleRate / this.targetRate;

    this.frame = new Int16Array(this.frameSamples);
    this.frameFill = 0;
    this.phase = 0;
    this.frameIndex = 0;

    // 8kHz-ish cutoff relative to the input Nyquist; 17 taps is cheap and
    // enough for a 3:1 or 2.76:1 decimation.
    this.taps = this.designLowpass(17, 0.5 / this.ratio);
    this.history = new Float32Array(this.taps.length);
    this.historyAt = 0;
  }

  designLowpass(length, cutoff) {
    const coeffs = new Float32Array(length);
    const mid = (length - 1) / 2;
    let sum = 0;
    for (let i = 0; i < length; i += 1) {
      const n = i - mid;
      const sinc = n === 0 ? 2 * cutoff : Math.sin(2 * Math.PI * cutoff * n) / (Math.PI * n);
      const hamming = 0.54 - 0.46 * Math.cos((2 * Math.PI * i) / (length - 1));
      coeffs[i] = sinc * hamming;
      sum += coeffs[i];
    }
    for (let i = 0; i < length; i += 1) coeffs[i] /= sum;
    return coeffs;
  }

  filter(sample) {
    const history = this.history;
    history[this.historyAt] = sample;
    this.historyAt = (this.historyAt + 1) % history.length;
    let acc = 0;
    const taps = this.taps;
    for (let i = 0; i < taps.length; i += 1) {
      acc += taps[i] * history[(this.historyAt + i) % history.length];
    }
    return acc;
  }

  process(inputs) {
    const channel = inputs[0] && inputs[0][0];
    if (!channel) return true;

    let energy = 0;
    let counted = 0;

    for (let i = 0; i < channel.length; i += 1) {
      const filtered = this.filter(channel[i]);
      this.phase += 1;
      if (this.phase < this.ratio) continue;
      this.phase -= this.ratio;

      const clamped = Math.max(-1, Math.min(1, filtered));
      this.frame[this.frameFill] = clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff;
      this.frameFill += 1;
      energy += clamped * clamped;
      counted += 1;

      if (this.frameFill === this.frameSamples) {
        const out = new Int16Array(this.frameSamples);
        out.set(this.frame);
        this.frameFill = 0;
        this.frameIndex += 1;

        const rms = counted > 0 ? Math.sqrt(energy / counted) : 0;
        // Levels only every 4th frame (≈12.5 Hz) — enough for a meter, cheap
        // enough not to churn React on every frame.
        const level = this.frameIndex % 4 === 0 ? rms : undefined;
        this.port.postMessage({ type: 'frame', data: out.buffer, level }, [out.buffer]);
        energy = 0;
        counted = 0;
      }
    }

    return true;
  }
}

registerProcessor('voce-capture', CaptureProcessor);
