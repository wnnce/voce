import type { Packet } from '@/realtime/packet';

export type ConnectionState =
  | 'idle'
  | 'creating'
  | 'connecting'
  | 'active'
  | 'reconnecting'
  | 'closing'
  | 'closed'
  | 'error';

/** Registered transport ids. Adding one means registering a provider. */
export type ConnectionKind = 'websocket' | 'demo';

export type ChatRole = 'user' | 'assistant';

export interface ChatMessage {
  id: string;
  role: ChatRole;
  text: string;
  final: boolean;
  /** Set when the server sent 0x11 while this bubble was still streaming. */
  interrupted?: boolean;
  at: number;
}

/** Drives the speaking indicator. `end` is undefined while the turn is open. */
export interface TimelineSpan {
  id: string;
  role: 'user' | 'agent';
  start: number;
  end?: number;
}

/** Minimal surface shared by every connection provider. */
export type { RealtimeConnection } from '@/realtime/connection';

export interface SessionConfig {
  workflowId: string;
  propertiesText: string;
  inputDeviceId: string;
  outputDeviceId: string;
  transport: ConnectionKind;
  /** Turn the microphone off to verify the downlink path on its own. */
  captureEnabled: boolean;
}

export type { Packet };
