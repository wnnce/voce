/**
 * Session lifecycle REST calls.
 *
 * Deliberately does not use `api/request.ts`: that helper surfaces every
 * failure as a global snackbar, while session errors must render inline in the
 * conversation area (see the design doc, §8.2). Status codes are preserved so
 * callers can distinguish "busy" from "expired" from "server down".
 */

const BASE_URL = import.meta.env.VITE_API_BASE_URL || '';

export class SessionApiError extends Error {
  status: number;
  /** 404 on a health check means the session was reaped by the idle reaper. */
  get expired() {
    return this.status === 404;
  }
  /** 409 means another connection already holds this session's lease. */
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
  let response: Response;
  try {
    response = await fetch(`${BASE_URL}${path}`, {
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
      // Not JSON; keep the raw body.
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

export const sessionApi = {
  /**
   * `name` is the workflow's **name**, not its id.
   *
   * SessionManager.CreateSession resolves the config through
   * WorkflowConfigManager.GetWithName, which indexes by `WorkflowConfig.Name`
   * (internal/engine/manager.go). Passing the id yields ErrWorkflowNotFound.
   */
  create: (workflowName: string, properties: Record<string, unknown>) =>
    call<{ session_id: string }>('/sessions', {
      method: 'POST',
      body: JSON.stringify({ name: workflowName, properties }),
    }),

  health: (sessionId: string) => call<SessionHealth>(`/sessions/health/${sessionId}`),

  renew: (sessionId: string) => call<null>(`/sessions/renew/${sessionId}`, { method: 'POST' }),

  destroy: (sessionId: string) => call<null>(`/sessions/${sessionId}`, { method: 'DELETE' }),
};

/** Rewrites the API base from http(s) to ws(s) for the realtime endpoint. */
export function realtimeURL(sessionId: string): string {
  const base = BASE_URL || window.location.origin;
  const wsBase = base.replace(/^http/, 'ws');
  return `${wsBase}/realtime/${sessionId}`;
}
