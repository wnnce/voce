import { useCallback, useEffect, useRef } from 'react';
import {
  PacketEncode,
  PacketType,
  decodeJSON,
  type CaptionPayload,
  type Packet,
} from '@/realtime/packet';
import {
  createConnection,
  describesConnection,
  type ConnectionHandlers,
  type RealtimeConnection,
} from '@/realtime';
import { sessionApi, SessionApiError } from '@/realtime/session';
import {
  ensureAudioContext,
  startCapture,
  startPlayback,
  type CaptureHandle,
  type PlaybackHandle,
} from '@/audio/engine';
import { requestMicrophone } from '@/audio/aec';
import { useSessionStore } from '@/store/useSessionStore';
import { useWorkflowStore } from '@/store/useAppStore';
import type { ConnectionState } from '@/types/session';

/**
 * The server reaps idle sessions after 60s and the cleanup ticker runs every
 * 30s. 20s leaves room for one dropped renew without losing the session.
 *
 * A websocket ping frame would be cheaper, but browsers cannot send control
 * frames and an empty binary frame dies in the server's Packet.Unmarshal before
 * it can refresh the lease.
 */
const KEEPALIVE_MS = 20_000;
const MAX_RECONNECT_ATTEMPTS = 5;

/**
 * The capture worklet emits 20ms frames; five of them make one upload packet.
 * Sending every 100ms instead of every 20ms cuts the packet count 5x with no
 * audible difference — the server buffers the stream either way.
 */
const FRAMES_PER_PACKET = 5;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

interface Controller {
  start: () => Promise<void>;
  stop: () => Promise<void>;
  busy: boolean;
}

/**
 * The session API keys on the workflow name, while the UI selects by id.
 * Resolve here rather than storing both: the config manager treats names as
 * unique (ErrWorkflowNameExists), ids as opaque.
 */
async function resolveWorkflowName(id: string): Promise<string | undefined> {
  const find = () => useWorkflowStore.getState().workflows.find((wf) => wf.id === id);
  let workflow = find();
  if (!workflow) {
    // Deep link or a hard refresh can land here before the list is loaded.
    await useWorkflowStore.getState().fetchWorkflows();
    workflow = find();
  }
  return workflow?.name;
}

export function useSessionController(): Controller {
  const store = useSessionStore;

  const connectionRef = useRef<RealtimeConnection | null>(null);
  const connectRef = useRef<(sessionId: string) => Promise<void>>(async () => {});
  const sessionIdRef = useRef<string | undefined>(undefined);
  const keepaliveRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const rafRef = useRef<number | undefined>(undefined);
  const captionQueue = useRef<CaptionPayload[]>([]);
  const tearingDownRef = useRef(false);

  const captureRef = useRef<CaptureHandle | null>(null);
  const playbackRef = useRef<PlaybackHandle | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const uplinkLevelRef = useRef(0);
  const uplinkBatch = useRef<Uint8Array[]>([]);
  const uplinkBatchBytes = useRef(0);

  // ---------------------------------------------------------------- captions

  const flushCaptions = useCallback(() => {
    rafRef.current = undefined;
    const pending = captionQueue.current.splice(0);
    const s = store.getState();
    for (const caption of pending) {
      s.upsertCaption(caption.role, caption.text, caption.is_final);
    }
  }, [store]);

  // --------------------------------------------------------------- keepalive

  const stopKeepalive = useCallback(() => {
    if (keepaliveRef.current !== undefined) {
      clearInterval(keepaliveRef.current);
      keepaliveRef.current = undefined;
    }
  }, []);

  const renew = useCallback(async () => {
    const id = sessionIdRef.current;
    if (!id) return;
    try {
      await sessionApi.renew(id);
    } catch (cause) {
      if (cause instanceof SessionApiError && cause.expired) {
        stopKeepalive();
        store.getState().setConnectionState('error', '会话已过期，请重新开始');
      }
    }
  }, [store, stopKeepalive]);

  const startKeepalive = useCallback(() => {
    if (!sessionIdRef.current) return;
    stopKeepalive();
    keepaliveRef.current = setInterval(() => {
      void renew();
    }, KEEPALIVE_MS);
  }, [renew, stopKeepalive]);

  // ------------------------------------------------------------------ audio

  const publishAudio = useCallback((payload: Uint8Array) => {
    const connection = connectionRef.current;
    if (!connection) return;
    connection.publish({ type: PacketType.Audio, encode: PacketEncode.Raw, payload });
  }, []);

  const onCaptureFrame = useCallback(
    (frame: ArrayBuffer) => {
      uplinkBatch.current.push(new Uint8Array(frame));
      uplinkBatchBytes.current += frame.byteLength;
      if (uplinkBatch.current.length < FRAMES_PER_PACKET) return;

      const merged = new Uint8Array(uplinkBatchBytes.current);
      let offset = 0;
      for (const part of uplinkBatch.current) {
        merged.set(part, offset);
        offset += part.byteLength;
      }
      uplinkBatch.current.length = 0;
      uplinkBatchBytes.current = 0;
      publishAudio(merged);
    },
    [publishAudio],
  );

  const onCaptureLevel = useCallback(
    (rms: number) => {
      uplinkLevelRef.current = rms;
      store.getState().setUplinkRms(rms);
    },
    [store],
  );

  const stopAudio = useCallback(() => {
    captureRef.current?.stop();
    captureRef.current = null;
    playbackRef.current?.close();
    playbackRef.current = null;
    for (const track of streamRef.current?.getTracks() ?? []) track.stop();
    streamRef.current = null;
    uplinkLevelRef.current = 0;
    uplinkBatch.current.length = 0;
    uplinkBatchBytes.current = 0;
    store.getState().setUplinkRms(0);
  }, [store]);

  const startAudio = useCallback(async () => {
    const config = store.getState().config;
    const ctx = await ensureAudioContext();

    playbackRef.current = startPlayback(ctx);
    if (config.outputDeviceId) {
      await playbackRef.current.setSink(config.outputDeviceId);
    }

    if (!config.captureEnabled) return;

    try {
      const stream = await requestMicrophone({ deviceId: config.inputDeviceId || undefined });
      streamRef.current = stream;
      captureRef.current = await startCapture(ctx, stream, onCaptureFrame, onCaptureLevel);
    } catch (cause) {
      store.getState().setError(`麦克风不可用：${(cause as Error).message}`);
      return;
    }

  }, [onCaptureFrame, onCaptureLevel, store]);

  // --------------------------------------------------------------- messages

  const handleMessage = useCallback(
    (packet: Packet) => {
      const s = store.getState();

      switch (packet.type) {
        case PacketType.Caption: {
          const caption = decodeJSON<CaptionPayload>(packet);
          if (!caption) return;
          captionQueue.current.push(caption);
          if (rafRef.current === undefined) {
            rafRef.current = requestAnimationFrame(flushCaptions);
          }
          return;
        }
        case PacketType.Audio:
          playbackRef.current?.push(packet.payload);
          return;
        case PacketType.Interrupter:
          // Server-driven interrupt. Everything already queued must go silent
          // now: the caption buffer, the playback ring, and the speaking state.
          // The worklet treats this as a single atomic flush, so no frame from
          // before the interrupt can leak out afterwards.
          if (rafRef.current !== undefined) {
            cancelAnimationFrame(rafRef.current);
            rafRef.current = undefined;
          }
          captionQueue.current.length = 0;
          playbackRef.current?.clear();
          s.markInterrupted();
          return;
        case PacketType.UserSpeechStart:
          s.openSpan('user');
          return;
        case PacketType.UserSpeechEnd:
          s.closeSpan('user');
          return;
        case PacketType.AgentSpeechStart:
          s.openSpan('agent');
          return;
        case PacketType.AgentSpeechEnd:
          s.closeSpan('agent');
          return;
        case PacketType.Close:
          stopKeepalive();
          // The session is over; nothing still queued should be heard.
          playbackRef.current?.clear();
          s.setConnectionState('closed');
          return;
        default:
          return;
      }
    },
    [flushCaptions, stopKeepalive, store],
  );

  // ------------------------------------------------------------- connection

  const handleState = useCallback(
    (state: ConnectionState, detail?: string) => {
      const s = store.getState();
      switch (state) {
        case 'active':
          s.setConnectionState('active');
          startKeepalive();
          return;
        case 'reconnecting':
          // Reconnection is driven by an effect watching the store, so the
          // handler stays a pure state transition.
          s.setConnectionState('reconnecting', detail);
          stopKeepalive();
          return;
        case 'error':
          s.setConnectionState('error', detail);
          stopKeepalive();
          return;
        default:
          s.setConnectionState(state, detail);
      }
    },
    [startKeepalive, stopKeepalive, store],
  );

  const handlers: ConnectionHandlers = {
    onMessage: handleMessage,
    onState: handleState,
    onMalformed: () => undefined,
  };

  // Connections hold their handlers for their whole lifetime; the trampoline
  // keeps them pointing at the latest render's callbacks.
  const handlersRef = useRef<ConnectionHandlers>(handlers);
  useEffect(() => {
    handlersRef.current = handlers;
  });

  const scheduleReconnect = useCallback(() => {
    if (tearingDownRef.current) return;
    if (reconnectTimerRef.current !== undefined) return;
    const id = sessionIdRef.current;
    if (!id) return;

    let attempt = 0;
    const attemptReconnect = async () => {
      reconnectTimerRef.current = undefined;
      attempt += 1;
      if (tearingDownRef.current || !sessionIdRef.current) return;

      try {
        await sessionApi.health(id);
      } catch (cause) {
        if (cause instanceof SessionApiError && cause.expired) {
          store.getState().setConnectionState('error', '会话已过期，请重新开始');
          return;
        }
      }

      if (attempt > MAX_RECONNECT_ATTEMPTS) {
        store.getState().setConnectionState('error', '重连失败，会话可能已关闭');
        return;
      }

      store.getState().setConnectionState('reconnecting', `第 ${attempt} 次重连`);
      await sleep(Math.min(1000 * attempt, 5000));
      if (tearingDownRef.current || !sessionIdRef.current) return;
      await connectRef.current(id);
    };

    void attemptReconnect();
  }, [store]);

  const connect = useCallback(
    async (sessionId: string) => {
      const transport = store.getState().config.transport;
      connectionRef.current?.close();

      const connection = createConnection(transport);
      connectionRef.current = connection;

      try {
        await connection.open({ sessionId: sessionId || undefined }, {
          onMessage: (packet) => handlersRef.current.onMessage(packet),
          onState: (state, detail) => handlersRef.current.onState(state, detail),
          onMalformed: () => handlersRef.current.onMalformed?.(),
        });
      } catch (cause) {
        store.getState().setConnectionState('error', (cause as Error).message);
        throw cause;
      }
    },
    [store],
  );

  useEffect(() => {
    connectRef.current = connect;
  }, [connect]);

  const connectionState = useSessionStore((state) => state.state);
  useEffect(() => {
    if (connectionState === 'reconnecting') scheduleReconnect();
  }, [connectionState, scheduleReconnect]);

  // ------------------------------------------------------------------- flow

  const dispose = useCallback(async () => {
    tearingDownRef.current = true;
    stopKeepalive();
    if (reconnectTimerRef.current !== undefined) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = undefined;
    }
    if (rafRef.current !== undefined) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = undefined;
    }
    captionQueue.current.length = 0;

    stopAudio();

    connectionRef.current?.close();
    connectionRef.current = null;

    const id = sessionIdRef.current;
    sessionIdRef.current = undefined;
    if (id) {
      await sessionApi.destroy(id).catch(() => undefined);
    }
  }, [stopAudio, stopKeepalive]);

  const start = useCallback(async () => {
    const s = store.getState();
    const { workflowId, propertiesText, transport } = s.config;

    if (!workflowId) {
      s.setError('请选择一个工作流');
      return;
    }

    let properties: Record<string, unknown>;
    try {
      const parsed: unknown = JSON.parse(propertiesText.trim() || '{}');
      if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('not an object');
      }
      properties = parsed as Record<string, unknown>;
    } catch {
      s.setError('覆盖参数必须是合法的 JSON 对象');
      return;
    }

    const provider = describesConnection(transport);
    if (!provider) {
      s.setError(`未注册的连接实现: ${transport}`);
      return;
    }

    tearingDownRef.current = false;
    sessionIdRef.current = undefined;
    s.reset();
    s.setConnectionState('creating');

    let sessionId = '';
    if (provider.requiresSession) {
      const workflowName = await resolveWorkflowName(workflowId);
      if (!workflowName) {
        store.getState().setConnectionState('error', `找不到工作流 ${workflowId}`);
        return;
      }
      try {
        const created = await sessionApi.create(workflowName, properties);
        sessionId = created.session_id;
        sessionIdRef.current = sessionId;
        store.getState().setSessionId(sessionId);
      } catch (cause) {
        const message =
          cause instanceof SessionApiError
            ? cause.status === 409
              ? '该会话已被其他连接占用'
              : cause.message
            : (cause as Error).message;
        store.getState().setConnectionState('error', message);
        return;
      }
    }

    try {
      await connect(sessionId);
    } catch {
      return; // connect() already published the error state
    }

    // Audio is only opened for transports that carry live traffic; the demo
    // provider replays scripted packets and needs no microphone permission.
    if (provider.requiresSession) {
      await startAudio().catch((cause: Error) => {
        store.getState().setError(cause.message);
      });
    }
  }, [connect, startAudio, store]);

  const stop = useCallback(async () => {
    store.getState().setConnectionState('closing');
    await dispose();
    store.getState().setConnectionState('closed');
  }, [dispose, store]);

  // Re-entering the tab restores throttled timers; refresh the lease at once.
  useEffect(() => {
    const onVisibility = () => {
      if (document.visibilityState === 'visible') void renew();
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [renew]);

  // Leaving the page tears the session down rather than leaving it to the idle
  // reaper, so a later start does not collide with a stale lease.
  useEffect(() => {
    return () => {
      void dispose();
    };
  }, [dispose]);

  const busy = useSessionStore(
    (state) =>
      state.state === 'creating' || state.state === 'connecting' || state.state === 'active',
  );

  return { start, stop, busy };
}
