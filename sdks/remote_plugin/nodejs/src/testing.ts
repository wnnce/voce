import type { Flow, PluginDefinition, PluginHandlers, PluginLogger, PluginContext } from '@/plugin.js'
import type { Signal, Payload } from '@/types.js'
import { z } from 'zod'

const ACTIVITY_BUFFER_SIZE = 1024

/**
 * MockFlow implements the Flow interface for testing purposes.
 * Captures emitted payloads and signals via hooks.
 */
export class MockFlow implements Flow {
  private readonly pingActivity?: () => void

  onSignalHook?: (port: number, signal: Signal) => void
  onPayloadHook?: (port: number, payload: Payload) => void

  constructor(pingActivity?: () => void) {
    this.pingActivity = pingActivity
  }

  async sendPayload(payload: Payload, options?: { port?: number }): Promise<void> {
    this.pingActivity?.()
    this.onPayloadHook?.(options?.port ?? 0, payload)
  }

  async sendSignal(signal: Signal, options?: { port?: number }): Promise<void> {
    this.pingActivity?.()
    this.onSignalHook?.(options?.port ?? 0, signal)
  }
}

const noopLogger: PluginLogger = {
  debug() {},
  info() {},
  warn() {},
  error() {},
}

/**
 * PluginTester provides a test harness for individual plugins defined via definePlugin.
 *
 * @example
 * ```ts
 * const tester = await PluginTester.create(myPlugin, { threshold: 0.8 })
 * tester.onPayload((port, payload) => { ... })
 * await tester.start()
 * await tester.injectPayload(createPayload('test', { key: 'val' }))
 * await tester.wait()
 * await tester.stop()
 * ```
 */
export class PluginTester {
  readonly definition: PluginDefinition<unknown>
  readonly handlers: PluginHandlers
  readonly mock: MockFlow

  private activityBuffer: Array<() => void> = []
  private activityWaiter: (() => void) | null = null

  private constructor(
    definition: PluginDefinition<unknown>,
    handlers: PluginHandlers,
  ) {
    this.definition = definition
    this.handlers = handlers
    this.mock = new MockFlow(() => this.pingActivity())
  }

  static async create<TConfig>(
    definition: PluginDefinition<TConfig>,
    config?: unknown,
  ): Promise<PluginTester> {
    const parsedConfig = definition.configSchema
      ? definition.configSchema.parse(config ?? {})
      : (config ?? {})

    const context: PluginContext<TConfig> = {
      instanceId: 'test-instance',
      logger: noopLogger,
      config: parsedConfig as TConfig,
    }

    const handlers = await definition.setup(context)
    return new PluginTester(definition as PluginDefinition<unknown>, handlers)
  }

  async start(): Promise<this> {
    await this.handlers.onStart?.(this.mock)
    await this.handlers.onReady?.(this.mock)
    return this
  }

  async stop(): Promise<void> {
    await this.handlers.onStop?.()
  }

  /**
   * Wait blocks until the plugin stops emitting activity for the specified duration.
   */
  async wait(timeoutMs = 100): Promise<void> {
    while (true) {
      const gotActivity = await Promise.race([
        this.waitForActivity(),
        sleep(timeoutMs).then(() => false as const),
      ])
      if (gotActivity === false) return
    }
  }

  async injectSignal(signal: Signal): Promise<this> {
    this.pingActivity()
    await this.handlers.onSignal?.(signal, this.mock)
    return this
  }

  async injectPayload(payload: Payload): Promise<this> {
    this.pingActivity()
    await this.handlers.onPayload?.(payload, this.mock)
    return this
  }

  onSignal(cb: (port: number, signal: Signal) => void): this {
    this.mock.onSignalHook = cb
    return this
  }

  onPayload(cb: (port: number, payload: Payload) => void): this {
    this.mock.onPayloadHook = cb
    return this
  }

  // ── private ──────────────────────────────────────────────────────────────

  private pingActivity(): void {
    if (this.activityWaiter) {
      const resolve = this.activityWaiter
      this.activityWaiter = null
      resolve()
      return
    }
    if (this.activityBuffer.length < ACTIVITY_BUFFER_SIZE) {
      this.activityBuffer.push(() => {})
    }
  }

  private waitForActivity(): Promise<true> {
    if (this.activityBuffer.length > 0) {
      this.activityBuffer.shift()
      return Promise.resolve(true)
    }
    return new Promise<true>((resolve) => {
      this.activityWaiter = () => resolve(true)
    })
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
