import { registerConnection } from '@/realtime/connection';
import { websocketProvider } from '@/realtime/providers/websocket';

/**
 * 内置传输协议注册。
 * 此处注册的顺序决定了 UI 连接选项列表的展示顺序。
 * 新增传输协议（如 Agora RTC）只需在此引入并调用 registerConnection。
 */
let registered = false;

export function registerBuiltinConnections(): void {
  if (registered) return;
  registered = true;
  registerConnection(websocketProvider);
}

registerBuiltinConnections();

export * from '@/realtime/connection';
export * from '@/realtime/packet';
export * from '@/realtime/session';
export { websocketProvider } from '@/realtime/providers/websocket';
