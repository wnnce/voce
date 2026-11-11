/**
 * 会话生命周期 REST API 调用。
 *
 * 不使用 `api/request.ts` 的全局错误提示弹窗（snackbar），因为会话级错误需直接呈现在对话视图内。
 * 保留原始 HTTP 状态码以便区分超时 (404)、冲突占用 (409) 与服务不可用等异常。
 */

const BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

function trimTrailingSlash(url: string): string {
  return url.replace(/\/+$/, '');
}

const cleanBaseUrl = trimTrailingSlash(BASE_URL);

export class SessionApiError extends Error {
  status: number;

  /** 404 表示会话已被服务端空闲清理机制回收 */
  get expired() {
    return this.status === 404;
  }

  /** 409 表示该会话 Lease 已被其他活跃连接持有 */
  get busy() {
    return this.status === 409;
  }

  constructor(status: number, message: string) {
    super(message);
    this.name = 'SessionApiError';
    this.status = status;
  }
}

interface ResultEnvelope<T> {
  code: number;
  message: string;
  data: T;
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  const url = `${cleanBaseUrl}${normalizedPath}`;
  let response: Response;

  try {
    response = await fetch(url, {
      ...init,
      headers: { 'Content-Type': 'application/json', ...init?.headers },
    });
  } catch (cause) {
    throw new SessionApiError(0, `无法连接服务端：${(cause as Error).message}`);
  }

  if (!response.ok) {
    const detail = await response.text().catch(() => '');
    let message = detail || response.statusText;
    try {
      const parsed = JSON.parse(detail) as { message?: string };
      if (parsed.message) message = parsed.message;
    } catch {
      // 保持原始文本错误
    }
    throw new SessionApiError(response.status, message || `HTTP ${response.status}`);
  }

  const body = (await response.json()) as ResultEnvelope<T>;
  if (body.code !== 200 && body.code !== 0) {
    throw new SessionApiError(response.status, body.message || '请求失败');
  }
  return body.data;
}

export interface SessionHealth {
  active_time: number;
  state: 'pending' | 'starting' | 'running' | 'paused' | 'stopped';
  created_at: string;
}

export interface WebRTCProviderInfo {
  name: string;
  label: string;
  description: string;
}

export interface WebRTCCredentials {
  provider?: string;
  app_id: string;
  pub_stream_id?: string;
  sub_stream_id?: string;
  token?: string;
  expire_at?: string;
  server_session_id?: string;
  server_track_name?: string;
  client_track_name?: string;
  base_url: string;
  stuns?: string[];
}

export const sessionApi = {
  /**
   * 获取支持的 WebRTC 提供者列表及默认提供者。
   */
  getWebRTCProviders: () =>
    call<{ default: string; providers: WebRTCProviderInfo[] }>('/webrtc/providers'),

  /**
   * 创建工作流会话。
   *
   * @param workflowName 工作流名称（非 ID，服务端 SessionManager 按 Name 索引）
   * @param properties 覆盖参数对象（JSON）
   */
  create: (workflowName: string, properties: Record<string, unknown>) =>
    call<{ session_id: string }>('/sessions', {
      method: 'POST',
      body: JSON.stringify({ name: workflowName, properties }),
    }),

  /**
   * 查询会话健康状态。
   */
  health: (sessionId: string) =>
    call<SessionHealth>(`/sessions/health/${encodeURIComponent(sessionId)}`),

  /**
   * 刷新/续期会话 Lease。
   */
  renew: (sessionId: string) =>
    call<null>(`/sessions/renew/${encodeURIComponent(sessionId)}`, { method: 'POST' }),

  /**
   * 销毁指定会话。
   */
  destroy: (sessionId: string) =>
    call<null>(`/sessions/${encodeURIComponent(sessionId)}`, { method: 'DELETE' }),

  /**
   * 获取 WebRTC 会话凭据与信令信息（可指定具体的 provider）。
   */
  attachWebRTC: (sessionId: string, provider?: string) => {
    const url = provider
      ? `/webrtc/${encodeURIComponent(sessionId)}/${encodeURIComponent(provider)}`
      : `/webrtc/${encodeURIComponent(sessionId)}`;
    return call<WebRTCCredentials>(url, { method: 'POST' });
  },
};

/**
 * 将 API Base URL 转换为 WebSocket 协议的 /realtime/{id} 地址。
 */
export function realtimeURL(sessionId: string): string {
  const base = cleanBaseUrl || window.location.origin;
  const wsBase = trimTrailingSlash(base.replace(/^http/, 'ws'));
  return `${wsBase}/realtime/${encodeURIComponent(sessionId)}`;
}
