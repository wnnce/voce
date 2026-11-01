import { create } from 'zustand';
import type {
  ChatMessage,
  ChatRole,
  ConnectionState,
  SessionConfig,
  TimelineSpan,
} from '@/types/session';

const SPAN_LIMIT = 24;

const defaultConfig: SessionConfig = {
  workflowId: '',
  propertiesText: '{}',
  inputDeviceId: '',
  outputDeviceId: '',
  transport: 'websocket',
  captureEnabled: true,
};

let seq = 0;
const nextId = (prefix: string) => `${prefix}-${Date.now().toString(36)}-${(seq += 1)}`;

interface SessionState {
  config: SessionConfig;
  state: ConnectionState;
  stateDetail?: string;
  sessionId?: string;
  error?: string;
  messages: ChatMessage[];
  spans: TimelineSpan[];
  uplinkRms: number;

  setConfig: (patch: Partial<SessionConfig>) => void;
  setConnectionState: (state: ConnectionState, detail?: string) => void;
  setSessionId: (id?: string) => void;
  setError: (message?: string) => void;
  setUplinkRms: (rms: number) => void;

  upsertCaption: (role: ChatRole, text: string, final: boolean) => void;
  markInterrupted: () => void;

  openSpan: (role: TimelineSpan['role']) => void;
  closeSpan: (role: TimelineSpan['role']) => void;

  reset: () => void;
}

export const useSessionStore = create<SessionState>((set) => ({
  config: defaultConfig,
  state: 'idle',
  messages: [],
  spans: [],
  uplinkRms: 0,

  setConfig: (patch) => set((s) => ({ config: { ...s.config, ...patch } })),

  setConnectionState: (state, stateDetail) =>
    set(() => ({ state, stateDetail, error: state === 'error' ? stateDetail : undefined })),

  setSessionId: (sessionId) => set(() => ({ sessionId })),

  setError: (error) => set(() => ({ error })),

  setUplinkRms: (uplinkRms) => set(() => ({ uplinkRms })),

  upsertCaption: (role, text, final) =>
    set((s) => {
      const messages = [...s.messages];
      const last = messages[messages.length - 1];
      // Server sends cumulative text, so an open bubble is overwritten rather
      // than appended to. A final caption seals it; the next one starts fresh.
      if (last && last.role === role && !last.final) {
        messages[messages.length - 1] = { ...last, text, final };
        return { messages };
      }
      messages.push({ id: nextId('msg'), role, text, final, at: Date.now() });
      return { messages };
    }),

  markInterrupted: () =>
    set((s) => {
      const messages = [...s.messages];
      for (let i = messages.length - 1; i >= 0; i -= 1) {
        if (messages[i].role === 'assistant' && !messages[i].final) {
          messages[i] = { ...messages[i], interrupted: true, final: true };
          break;
        }
      }
      const spans = s.spans.map((span) =>
        span.role === 'agent' && span.end === undefined ? { ...span, end: Date.now() } : span,
      );
      return { messages, spans };
    }),

  openSpan: (role) =>
    set((s) => {
      if (s.spans.some((span) => span.role === role && span.end === undefined)) return {};
      const spans = [...s.spans, { id: nextId(role), role, start: Date.now() }];
      return { spans: spans.slice(-SPAN_LIMIT) };
    }),

  closeSpan: (role) =>
    set((s) => ({
      spans: s.spans.map((span) =>
        span.role === role && span.end === undefined ? { ...span, end: Date.now() } : span,
      ),
    })),

  reset: () =>
    set(() => ({
      state: 'idle',
      stateDetail: undefined,
      sessionId: undefined,
      error: undefined,
      messages: [],
      spans: [],
      uplinkRms: 0,
    })),
}));
