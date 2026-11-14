import type { Packet } from '@/realtime/packet';
import type { ConnectionKind, ConnectionState } from '@/types/session';

/**
 * 实时连接层核心契约。
 *
 * 页面层与控制器仅通过此抽象与底层通讯：
 * - 连接通过 `handlers.onMessage` 推送已解码的入站 VC 封包
 * - 页面/控制器通过 `publish` 发送出站封包
 * - 连接生命周期通过 `handlers.onState` 同步
 *
 * 新增传输方式只需注册一个 `ConnectionProvider`，无需改动状态管理与业务视图。
 */

export type { ConnectionKind };

export interface ConnectionTarget {
  /** 会话 ID，附加到后端 Session 时必填 */
  sessionId?: string;
  /** 可选输入音频设备 ID */
  inputDeviceId?: string;
  /** 可选输出音频设备 ID */
  outputDeviceId?: string;
  /** 是否启用麦克风采集（默认 true） */
  captureEnabled?: boolean;
  /** 可选指定的 WebRTC provider 名称，如 'wtn', 'cloudflare' */
  webrtcProvider?: string;
}

export interface ConnectionHandlers {
  /** 已校验并完成解码的入站数据包 */
  onMessage: (packet: Packet) => void;
  /** 连接状态变更通知。进入 'active' 状态后方可进行有效 publish */
  onState: (state: ConnectionState, detail?: string) => void;
  /** 格式错误帧回调。连接仍然保持，通知上层记录或提示 */
  onMalformed?: () => void;
  /** 可选麦克风输入音量回调 (0.0 ~ 1.0) */
  onUplinkLevel?: (level: number) => void;
}

export interface RealtimeConnection {
  /** 当前连接类型标识 */
  readonly kind: ConnectionKind;
  /** 建立并启动连接。成功后进入 active 状态，失败时 reject */
  open(target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void>;
  /** 发送出站数据包。在非 active 状态下调用安全丢弃，不阻塞 */
  publish(packet: Packet): void;
  /** 主动断开并释放所有底层网络资源与监听器 */
  close(): void;
}

export interface ConnectionProvider {
  /** 唯一标识 */
  readonly kind: ConnectionKind;
  /** UI 显示名称 */
  readonly label: string;
  /** UI 描述信息 */
  readonly description: string;
  /** 是否需要服务端先通过 REST 创建 Session 才能连接 */
  readonly requiresSession: boolean;
  /** 是否自管理原生音频轨道（采集与播放），为 true 时上层控制器跳过独立 AudioWorklet 管道 */
  readonly managesAudio?: boolean;
  /** 创建连接实例工厂方法 */
  create: () => RealtimeConnection;
}

const providers = new Map<ConnectionKind, ConnectionProvider>();

/**
 * 注册连接实现提供者。
 */
export function registerConnection(provider: ConnectionProvider): void {
  providers.set(provider.kind, provider);
}

/**
 * 注销指定的连接实现。
 */
export function unregisterConnection(kind: ConnectionKind): boolean {
  return providers.delete(kind);
}

/**
 * 检查指定连接实现是否已注册。
 */
export function hasConnection(kind: ConnectionKind): boolean {
  return providers.has(kind);
}

/**
 * 获取当前已注册的所有连接实现列表（按注册顺序排列）。
 */
export function listConnections(): ConnectionProvider[] {
  return [...providers.values()];
}

/**
 * 根据类型创建新的实时连接实例。
 */
export function createConnection(kind: ConnectionKind): RealtimeConnection {
  const provider = providers.get(kind);
  if (!provider) {
    throw new Error(`未注册的连接实现: ${kind}`);
  }
  return provider.create();
}

/**
 * 获取指定连接类型的元数据描述。
 */
export function describesConnection(kind: ConnectionKind): ConnectionProvider | undefined {
  return providers.get(kind);
}
