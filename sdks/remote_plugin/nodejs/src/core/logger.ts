import type { AsyncQueue } from '../utils/async-queue.js'
import type { RuntimeMessage } from '../proto/plugin.js'
import { LogLevel, RuntimeMessageType } from '../proto/plugin.js'

// ---------------------------------------------------------------------------
// Log level mapping
// ---------------------------------------------------------------------------

const LOG_LEVEL_MAP: Record<string, LogLevel> = {
  debug: LogLevel.LOG_LEVEL_DEBUG,
  info: LogLevel.LOG_LEVEL_INFO,
  warn: LogLevel.LOG_LEVEL_WARN,
  error: LogLevel.LOG_LEVEL_ERROR,
}

export function mapLogLevel(level: string): LogLevel {
  return LOG_LEVEL_MAP[level.toLowerCase()] ?? LogLevel.LOG_LEVEL_UNSPECIFIED
}

// ---------------------------------------------------------------------------
// LogMessage type
// ---------------------------------------------------------------------------

export interface LogMessage {
  level: LogLevel
  message: string
}

// ---------------------------------------------------------------------------
// RemoteLogHandler
// ---------------------------------------------------------------------------

/**
 * Buffers log messages and forwards them to a gRPC output queue.
 *
 * Logs that arrive when the buffer is full are silently dropped
 * to prevent memory growth in the fast path.
 */
export class RemoteLogHandler {
  private readonly queue: LogMessage[] = []
  private readonly maxSize: number
  private waiter: ((msg: LogMessage) => void) | null = null
  private closed = false

  constructor(maxSize = 1000) {
    this.maxSize = maxSize
  }

  /** Enqueue a log entry. Non-blocking; drops if full. */
  enqueue(level: LogLevel, message: string): void {
    if (this.closed) return

    const msg: LogMessage = { level, message }

    if (this.waiter) {
      const resolve = this.waiter
      this.waiter = null
      resolve(msg)
      return
    }

    if (this.queue.length >= this.maxSize) return // drop
    this.queue.push(msg)
  }

  /**
   * Continuously read log messages and push them as RuntimeMessages
   * to the provided output queue.
   *
   * Stops when an AbortSignal is triggered.
   */
  async readLoop(
    output: AsyncQueue<RuntimeMessage>,
    makeMsg: (data: LogMessage) => RuntimeMessage,
    signal: AbortSignal,
  ): Promise<void> {
    while (!signal.aborted) {
      const msg = await this.dequeue(signal)
      if (!msg) break
      output.put(makeMsg(msg))
    }
  }

  close(): void {
    this.closed = true
    if (this.waiter) {
      // Resolve the pending dequeue with nothing so readLoop can exit
      const resolve = this.waiter
      this.waiter = null
      // We need to resolve, but there's no message. We'll use a dummy
      // that readLoop will ignore since `closed` is now true.
      resolve(null as unknown as LogMessage)
    }
  }

  // ── private ──────────────────────────────────────────────────────────────

  private dequeue(signal: AbortSignal): Promise<LogMessage | null> {
    if (this.queue.length > 0) {
      return Promise.resolve(this.queue.shift()!)
    }
    if (this.closed || signal.aborted) {
      return Promise.resolve(null)
    }
    return new Promise<LogMessage | null>((resolve) => {
      const onAbort = () => {
        this.waiter = null
        resolve(null)
      }
      signal.addEventListener('abort', onAbort, { once: true })
      this.waiter = (msg: LogMessage) => {
        signal.removeEventListener('abort', onAbort)
        resolve(msg)
      }
    })
  }
}

// ---------------------------------------------------------------------------
// PluginLogger — injected into each plugin instance
// ---------------------------------------------------------------------------

/**
 * Creates a PluginLogger that forwards logs to a RemoteLogHandler.
 */
export function createPluginLogger(handler: RemoteLogHandler, minLevel: LogLevel = LogLevel.LOG_LEVEL_INFO) {
  const shouldLog = (level: LogLevel) => level >= minLevel

  return {
    debug(message: string) {
      if (shouldLog(LogLevel.LOG_LEVEL_DEBUG)) handler.enqueue(LogLevel.LOG_LEVEL_DEBUG, message)
    },
    info(message: string) {
      if (shouldLog(LogLevel.LOG_LEVEL_INFO)) handler.enqueue(LogLevel.LOG_LEVEL_INFO, message)
    },
    warn(message: string) {
      if (shouldLog(LogLevel.LOG_LEVEL_WARN)) handler.enqueue(LogLevel.LOG_LEVEL_WARN, message)
    },
    error(message: string) {
      if (shouldLog(LogLevel.LOG_LEVEL_ERROR)) handler.enqueue(LogLevel.LOG_LEVEL_ERROR, message)
    },
  }
}

/**
 * Build a RuntimeMessage factory for log messages.
 */
export function createLogMessageBuilder(instanceId: string) {
  return (data: LogMessage): RuntimeMessage => ({
    instanceId,
    messageId: crypto.randomUUID().replace(/-/g, ''),
    correlationId: '',
    type: RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_LOG,
    metadata: {},
    body: {
      $case: 'emitLog' as const,
      emitLog: {
        level: data.level,
        message: data.message,
        fields: {},
      },
    },
  })
}
