import type { Packet } from '@/realtime/packet';
import type { ConnectionKind, ConnectionState } from '@/types/session';

/**
 * Connection layer contract.
 *
 * Nothing above this layer knows which transport is in use: the connection
 * pushes inbound packets through `handlers.onMessage`, and the page pushes
 * outbound packets through `publish`. Adding a transport means registering a
 * provider — no changes in the controller, the store, or the page.
 */

export type { ConnectionKind };

export interface ConnectionTarget {
  /** Required by transports that attach to an existing session lease. */
  sessionId?: string;
}

export interface ConnectionHandlers {
  /** Inbound packet, already decoded and validated. */
  onMessage: (packet: Packet) => void;
  /** Transport lifecycle. 'active' means publish() is legal. */
  onState: (state: ConnectionState, detail?: string) => void;
  /** Frame that failed validation. The connection stays open. */
  onMalformed?: () => void;
}

export interface RealtimeConnection {
  readonly kind: ConnectionKind;
  open(target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void>;
  /** Outbound packet. Safe to call before 'active'; frames are dropped. */
  publish(packet: Packet): void;
  close(): void;
}

export interface ConnectionProvider {
  kind: ConnectionKind;
  label: string;
  description: string;
  /** true when the provider needs a REST-created session before opening. */
  requiresSession: boolean;
  create: () => RealtimeConnection;
}

const providers = new Map<ConnectionKind, ConnectionProvider>();

export function registerConnection(provider: ConnectionProvider): void {
  providers.set(provider.kind, provider);
}

/** Descriptors for UI rendering, in registration order. */
export function listConnections(): ConnectionProvider[] {
  return [...providers.values()];
}

export function createConnection(kind: ConnectionKind): RealtimeConnection {
  const provider = providers.get(kind);
  if (!provider) {
    throw new Error(`未注册的连接实现: ${kind}`);
  }
  return provider.create();
}

export function describesConnection(kind: ConnectionKind): ConnectionProvider | undefined {
  return providers.get(kind);
}
