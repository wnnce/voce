import { decodePacket, encodePacket, type Packet } from '@/realtime/packet';
import type {
  ConnectionHandlers,
  ConnectionProvider,
  ConnectionTarget,
  RealtimeConnection,
} from '@/realtime/connection';
import { realtimeURL } from '@/realtime/session';

class WebSocketConnection implements RealtimeConnection {
  readonly kind = 'websocket' as const;

  private socket: WebSocket | null = null;
  private closed = false;

  open(target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void> {
    const { sessionId } = target;
    if (!sessionId) {
      return Promise.reject(new Error('websocket 连接需要 sessionId'));
    }

    this.closed = false;

    return new Promise((resolve, reject) => {
      const socket = new WebSocket(realtimeURL(sessionId));
      this.socket = socket;
      socket.binaryType = 'arraybuffer';

      socket.onopen = () => {
        if (this.closed) return;
        handlers.onState('active');
        resolve();
      };

      socket.onmessage = (event: MessageEvent<ArrayBuffer | string>) => {
        if (typeof event.data === 'string') {
          handlers.onMalformed?.();
          return;
        }
        const { packet } = decodePacket(event.data);
        if (!packet) {
          handlers.onMalformed?.();
          return;
        }
        handlers.onMessage(packet);
      };

      socket.onerror = () => {
        if (this.closed) return;
        handlers.onState('error', '网络错误');
        reject(new Error('websocket error'));
      };

      socket.onclose = (event: CloseEvent) => {
        if (this.closed) return;
        if (event.code === 1008) {
          handlers.onState('error', '会话被其他连接占用');
          return;
        }
        handlers.onState('reconnecting', event.reason || `连接关闭 (${event.code})`);
      };
    });
  }

  publish(packet: Packet): void {
    const socket = this.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    socket.send(encodePacket(packet));
  }

  close(): void {
    this.closed = true;
    const socket = this.socket;
    this.socket = null;
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      socket.close(1000);
    }
  }
}

export const websocketProvider: ConnectionProvider = {
  kind: 'websocket',
  label: 'WebSocket',
  description: '二进制 VC 封包，经 /realtime/{id}',
  requiresSession: true,
  create: () => new WebSocketConnection(),
};
