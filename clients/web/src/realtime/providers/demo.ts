import { PacketEncode, PacketType, jsonPayload, type Packet } from '@/realtime/packet';
import type {
  ConnectionHandlers,
  ConnectionProvider,
  ConnectionTarget,
  RealtimeConnection,
} from '@/realtime/connection';

interface ScriptStep {
  delay: number;
  packet: Packet;
}

const caption = (role: 'user' | 'assistant', text: string, isFinal: boolean): Packet => ({
  type: PacketType.Caption,
  encode: PacketEncode.Json,
  payload: jsonPayload({ text, role, is_final: isFinal }),
});

const signal = (type: number): Packet => ({
  type,
  encode: PacketEncode.Raw,
  payload: new Uint8Array(0),
});

/** 20ms of 16kHz mono PCM16 silence — drives playback and latency metrics. */
const audioFrame = (): Packet => ({
  type: PacketType.Audio,
  encode: PacketEncode.Raw,
  payload: new Uint8Array(640),
});

/** Cumulative text, mirroring the caption plugin's builder semantics. */
function userTurn(text: string): ScriptStep[] {
  const chars = [...text];
  const chunk = Math.max(1, Math.floor(chars.length / 3));
  const steps: ScriptStep[] = [{ delay: 0, packet: signal(PacketType.UserSpeechStart) }];
  let acc = '';
  for (let i = 0; i < chars.length; i += chunk) {
    acc += chars.slice(i, i + chunk).join('');
    steps.push({ delay: 220, packet: caption('user', acc, false) });
  }
  steps.push({ delay: 380, packet: caption('user', acc, true) });
  steps.push({ delay: 60, packet: signal(PacketType.UserSpeechEnd) });
  return steps;
}

function agentTurn(text: string, interrupted: boolean): ScriptStep[] {
  const chars = [...text];
  const chunk = Math.max(1, Math.ceil(chars.length / 6));
  const steps: ScriptStep[] = [{ delay: 260, packet: signal(PacketType.AgentSpeechStart) }];
  let acc = '';
  let audioSent = 0;
  for (let i = 0; i < chars.length; i += chunk) {
    acc += chars.slice(i, i + chunk).join('');
    steps.push({ delay: 200, packet: caption('assistant', acc, false) });
    if (audioSent < 3) {
      steps.push({ delay: 0, packet: audioFrame() });
      audioSent += 1;
    }
  }
  if (interrupted) {
    steps.push({ delay: 260, packet: signal(PacketType.Interrupter) });
    return steps;
  }
  steps.push({ delay: 200, packet: caption('assistant', acc, true) });
  for (let i = 0; i < 12; i += 1) {
    steps.push({ delay: 20, packet: audioFrame() });
  }
  steps.push({ delay: 120, packet: signal(PacketType.AgentSpeechEnd) });
  return steps;
}

const ROUNDS: ScriptStep[][] = [
  agentTurn('北京今天晴，气温 12 到 22 度。', false),
  agentTurn('它会把语音先转成文字，再交给语言模型，最后合成语音。', true),
  agentTurn('抱歉打断我了，你想问的是哪一部分？', false),
];

/**
 * Scripted provider: same packet shapes and ordering as the real backend, so
 * every consumer (captions, timeline, latency metrics, packet log) runs through
 * the production path. Used by the P0 stage and for UI work without a backend.
 */
class DemoConnection implements RealtimeConnection {
  readonly kind = 'demo' as const;

  private handlers: ConnectionHandlers | null = null;
  private timers: ReturnType<typeof setTimeout>[] = [];
  private closed = false;
  private round = 0;
  private resolveOpen: (() => void) | null = null;

  open(_target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void> {
    this.handlers = handlers;
    this.closed = false;
    this.round = 0;

    return new Promise((resolve) => {
      this.resolveOpen = resolve;
      this.handlers?.onState('connecting', '演示数据');
      this.later(() => {
        if (this.closed) return;
        this.handlers?.onState('active', '演示数据');
        this.resolveOpen?.();
        this.resolveOpen = null;
        this.play(userTurn('今天天气怎么样？'), () => this.nextRound());
      }, 400);
    });
  }

  publish(): void {
    // Demo mode has no backend to answer; uplink packets are dropped.
  }

  close(): void {
    this.closed = true;
    for (const timer of this.timers) clearTimeout(timer);
    this.timers.length = 0;
    this.handlers?.onState('closed');
    this.handlers = null;
    this.resolveOpen = null;
  }

  private later(fn: () => void, delay: number): void {
    this.timers.push(setTimeout(fn, delay));
  }

  private play(steps: ScriptStep[], done: () => void): void {
    let elapsed = 0;
    for (const step of steps) {
      elapsed += step.delay;
      this.later(() => {
        if (this.closed) return;
        this.handlers?.onMessage(step.packet);
      }, elapsed);
    }
    this.later(() => {
      if (!this.closed) done();
    }, elapsed + 900);
  }

  private nextRound(): void {
    if (this.closed) return;
    const steps = ROUNDS[this.round % ROUNDS.length];
    this.round += 1;
    this.play(steps, () => this.nextRound());
  }
}

export const demoProvider: ConnectionProvider = {
  kind: 'demo',
  label: '演示数据',
  description: '脚本包驱动，不连服务端',
  requiresSession: false,
  create: () => new DemoConnection(),
};
