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
  private pendingOpenReject: ((err: Error) => void) | null = null;

  open(target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void> {
    const { sessionId } = target;
    if (!sessionId) {
      return Promise.reject(new Error('WebSocket 连接需要 sessionId'));
    }

    this.closed = false;

    return new Promise((resolve, reject) => {
      let settled = false;

      const finishSuccess = () => {
        if (settled) return;
        settled = true;
        this.pendingOpenReject = null;
        resolve();
      };

      const finishFailure = (err: Error) => {
        if (settled) return;
        settled = true;
        this.pendingOpenReject = null;
        reject(err);
      };

      this.pendingOpenReject = finishFailure;

      let socket: WebSocket;
      try {
        socket = new WebSocket(realtimeURL(sessionId));
      } catch (cause) {
        finishFailure(cause instanceof Error ? cause : new Error(String(cause)));
        return;
      }

      this.socket = socket;
      socket.binaryType = 'arraybuffer';

      socket.onopen = () => {
        if (this.closed) return;
        handlers.onState('active');
        finishSuccess();
      };

      socket.onmessage = (event: MessageEvent<ArrayBuffer | string>) => {
        if (this.closed) return;
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
        finishFailure(new Error('WebSocket 网络连接失败'));
      };

      socket.onclose = (event: CloseEvent) => {
        if (this.closed) return;
        this.detachListeners(socket);
        this.socket = null;

        const reason = event.reason || `连接关闭 (${event.code})`;
        finishFailure(new Error(reason));

        if (event.code === 1008) {
          handlers.onState('error', '会话被其他连接占用');
          return;
        }

        handlers.onState('reconnecting', reason);
      };
    });
  }

  publish(packet: Packet): void {
    const socket = this.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    try {
      socket.send(encodePacket(packet));
    } catch (err) {
      console.warn('[WebSocketConnection] 数据发送失败:', err);
    }
  }

  close(): void {
    this.closed = true;

    if (this.pendingOpenReject) {
      this.pendingOpenReject(new Error('连接已手动取消'));
      this.pendingOpenReject = null;
    }

    const socket = this.socket;
    this.socket = null;

    if (socket) {
      this.detachListeners(socket);
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
        socket.close(1000, '客户端正常断开');
      }
    }
  }

  private detachListeners(socket: WebSocket): void {
    socket.onopen = null;
    socket.onmessage = null;
    socket.onerror = null;
    socket.onclose = null;
  }
}

export const websocketProvider: ConnectionProvider = {
  kind: 'websocket',
  label: 'WebSocket',
  description: '二进制 VC 封包，经 /realtime/{id}',
  requiresSession: true,
  create: () => new WebSocketConnection(),
};
