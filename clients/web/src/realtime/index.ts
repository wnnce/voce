import { registerConnection } from '@/realtime/connection';
import { demoProvider } from '@/realtime/providers/demo';
import { websocketProvider } from '@/realtime/providers/websocket';

/**
 * Built-in transport registrations. Order here is the order shown in the UI.
 * Adding a transport is a one-line change in this file plus its provider.
 */
let registered = false;

export function registerBuiltinConnections(): void {
  if (registered) return;
  registered = true;
  registerConnection(websocketProvider);
  registerConnection(demoProvider);
}

registerBuiltinConnections();

export * from '@/realtime/connection';
