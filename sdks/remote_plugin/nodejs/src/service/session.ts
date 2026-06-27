import { AsyncLocalStorage } from 'node:async_hooks'
import type { Flow } from '../core/flow.js'
import type { AsyncPlugin } from '../core/plugin.js'
import { Signal } from '../schema/signal.js'
import { Payload } from '../schema/payload.js'
import { AsyncQueue } from '../utils/async-queue.js'
import {
  LifecycleType,
  ReportStatus,
  RuntimeMessageType,
  type RuntimeMessage,
} from '../proto/plugin.js'

// ---------------------------------------------------------------------------
// Correlation ID context (AsyncLocalStorage)
// ---------------------------------------------------------------------------

const correlationIdStorage = new AsyncLocalStorage<string>()

export function getCurrentCorrelationId(): string {
  return correlationIdStorage.getStore() ?? ''
}

// ---------------------------------------------------------------------------
// PluginSession
// ---------------------------------------------------------------------------

/**
 * Manages a single RunInstance bidirectional stream for one plugin instance.
 *
 * Implements the Flow interface so that plugins can emit payloads/signals
 * directly through the session's outgoing message queue.
 */
export class PluginSession implements Flow {
  readonly instanceId: string
  readonly outgoingMessages: AsyncQueue<RuntimeMessage>

  private readonly plugin: AsyncPlugin<unknown>
  private readonly ackIntervalMs: number
  private closed = false
  private readonly runningTasks = new Map<string, AbortController>()

  constructor(
    instanceId: string,
    plugin: AsyncPlugin<unknown>,
    outgoingMessages: AsyncQueue<RuntimeMessage>,
    options: { ackIntervalSec: number },
  ) {
    this.instanceId = instanceId
    this.plugin = plugin
    this.outgoingMessages = outgoingMessages
    this.ackIntervalMs = options.ackIntervalSec * 1000
  }

  // ── Flow interface ───────────────────────────────────────────────────────

  async sendPayload(payload: Payload, options?: { port?: number }): Promise<void> {
    const msg = this.newRuntimeMessage(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD)
    msg.body = {
      $case: 'emitPayload' as const,
      emitPayload: {
        payload: {
          name: payload.name,
          properties: payload.toJsonBytes(),
        },
        port: options?.port ?? 0,
      },
    }
    this.outgoingMessages.put(msg)
  }

  async sendSignal(signal: Signal, options?: { port?: number }): Promise<void> {
    const msg = this.newRuntimeMessage(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL)
    msg.body = {
      $case: 'emitSignal' as const,
      emitSignal: {
        signal: {
          name: signal.name,
          properties: signal.toJsonBytes(),
        },
        port: options?.port ?? 0,
      },
    }
    this.outgoingMessages.put(msg)
  }

  // ── Stream processing ────────────────────────────────────────────────────

  /**
   * Reads incoming RuntimeMessages from the gRPC stream and dispatches
   * them to the plugin. Each message is handled in its own "task" (Promise)
   * tracked by correlation_id, allowing cancel propagation.
   */
  async processStream(messages: AsyncIterable<RuntimeMessage>): Promise<void> {
    try {
      for await (const message of messages) {
        if (message.type === RuntimeMessageType.RUNTIME_MESSAGE_TYPE_CANCEL) {
          this.handleCancel(message)
          continue
        }
        this.startMessageTask(message)
      }
    } catch (err) {
      if (!isAbortError(err)) {
        console.error(
          `remote plugin runtime stream failed instance_id=${this.instanceId}`,
          err,
        )
      }
    } finally {
      await this.drainRunningTasks()
      await this.close()
    }
  }

  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true

    // Cancel all running tasks
    for (const ac of this.runningTasks.values()) {
      ac.abort()
    }
    this.runningTasks.clear()

    // Signal the output queue to terminate
    this.outgoingMessages.put(null)
  }

  // ── private: task management ─────────────────────────────────────────────

  private startMessageTask(message: RuntimeMessage): void {
    const correlationId = message.messageId
    const ac = new AbortController()
    this.runningTasks.set(correlationId, ac)

    this.handleRuntimeMessage(message, ac.signal)
      .catch(() => {}) // errors handled inside handleRuntimeMessage
      .finally(() => {
        this.runningTasks.delete(correlationId)
      })
  }

  private handleCancel(message: RuntimeMessage): void {
    const correlationId = message.correlationId
    if (!correlationId) return
    const ac = this.runningTasks.get(correlationId)
    ac?.abort()
  }

  private async handleRuntimeMessage(
    message: RuntimeMessage,
    signal: AbortSignal,
  ): Promise<void> {
    const correlationId = message.messageId
    let ackTimer: ReturnType<typeof setInterval> | undefined

    try {
      // Send initial ACK
      this.sendAck(correlationId)

      // Start ACK keepalive
      ackTimer = setInterval(() => this.sendAck(correlationId), this.ackIntervalMs)

      // Run dispatch in correlation context
      await correlationIdStorage.run(correlationId, () =>
        this.dispatchMessage(message, signal),
      )

      // Success report
      this.sendReport(correlationId, ReportStatus.REPORT_STATUS_OK)
    } catch (err) {
      if (isAbortError(err)) {
        console.info(
          `remote plugin task canceled instance_id=${this.instanceId} correlation_id=${correlationId}`,
        )
        this.sendReport(correlationId, ReportStatus.REPORT_STATUS_CANCELED)
      } else {
        console.error(
          `remote plugin event failed instance_id=${this.instanceId} message_id=${message.messageId} type=${message.type}`,
          err,
        )
        this.sendReport(
          correlationId,
          ReportStatus.REPORT_STATUS_ERROR,
          err instanceof Error ? err : new Error(String(err)),
        )
      }
    } finally {
      if (ackTimer) clearInterval(ackTimer)
    }
  }

  private async dispatchMessage(
    message: RuntimeMessage,
    signal: AbortSignal,
  ): Promise<void> {
    const body = message.body
    if (!body) throw new Error('runtime message body is empty')

    // Check abort before dispatching
    throwIfAborted(signal)

    switch (body.$case) {
      case 'lifecycle':
        await this.dispatchLifecycle(body.lifecycle.type)
        break
      case 'signal':
        await this.plugin.onSignal(
          this,
          Signal.fromJsonBytes(body.signal.name, body.signal.properties),
        )
        break
      case 'payload':
        await this.plugin.onPayload(
          this,
          Payload.fromJsonBytes(body.payload.name, body.payload.properties),
        )
        break
      default:
        throw new Error(`unsupported runtime message body: ${(body as { $case: string }).$case}`)
    }
  }

  private async dispatchLifecycle(type: LifecycleType): Promise<void> {
    switch (type) {
      case LifecycleType.LIFECYCLE_TYPE_START:
        await this.plugin.onStart(this)
        break
      case LifecycleType.LIFECYCLE_TYPE_READY:
        await this.plugin.onReady(this)
        break
      case LifecycleType.LIFECYCLE_TYPE_PAUSE:
        await this.plugin.onPause()
        break
      case LifecycleType.LIFECYCLE_TYPE_RESUME:
        await this.plugin.onResume(this)
        break
      case LifecycleType.LIFECYCLE_TYPE_STOP:
        await this.plugin.onStop()
        break
      default:
        throw new Error(`unsupported lifecycle type: ${type}`)
    }
  }

  // ── private: message builders ────────────────────────────────────────────

  private newRuntimeMessage(type: RuntimeMessageType, correlationId?: string): RuntimeMessage {
    return {
      instanceId: this.instanceId,
      messageId: crypto.randomUUID().replace(/-/g, ''),
      correlationId: correlationId ?? getCurrentCorrelationId(),
      type,
      metadata: {},
      body: undefined,
    }
  }

  private sendAck(correlationId: string): void {
    const msg = this.newRuntimeMessage(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_ACK, correlationId)
    msg.body = {
      $case: 'ack' as const,
      ack: { timestamp: Date.now() },
    }
    this.outgoingMessages.put(msg)
  }

  private sendReport(
    correlationId: string,
    status: ReportStatus,
    error?: Error,
  ): void {
    const msg = this.newRuntimeMessage(
      RuntimeMessageType.RUNTIME_MESSAGE_TYPE_REPORT,
      correlationId,
    )
    msg.body = {
      $case: 'report' as const,
      report: {
        status,
        error: error
          ? { code: error.constructor.name, message: error.message, details: '' }
          : undefined,
      },
    }
    this.outgoingMessages.put(msg)
  }

  // ── private: cleanup ─────────────────────────────────────────────────────

  private async drainRunningTasks(): Promise<void> {
    const tasks: Promise<void>[] = []
    for (const [, ac] of this.runningTasks) {
      ac.abort()
    }
    // Wait for all tasks to settle
    await Promise.allSettled(tasks)
    this.runningTasks.clear()
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
    || err instanceof Error && err.name === 'AbortError'
}

function throwIfAborted(signal: AbortSignal): void {
  if (signal.aborted) {
    throw new DOMException('The operation was aborted', 'AbortError')
  }
}
