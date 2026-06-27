import type { Flow } from './flow.js'
import type { AsyncPlugin } from './plugin.js'
import { Signal } from '../schema/signal.js'
import { Payload } from '../schema/payload.js'

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

/**
 * PluginTester provides a test harness for individual plugins.
 *
 * @example
 * ```ts
 * const tester = new PluginTester(new MyPlugin(config))
 * tester.onPayload((port, payload) => { ... })
 * await tester.start()
 * await tester.injectPayload(new Payload('test', { key: 'val' }))
 * await tester.wait()
 * await tester.stop()
 * ```
 */
export class PluginTester {
  readonly plugin: AsyncPlugin<unknown>
  readonly mock: MockFlow

  private activityBuffer: Array<() => void> = []
  private activityWaiter: (() => void) | null = null

  constructor(plugin: AsyncPlugin<unknown>) {
    this.plugin = plugin
    this.mock = new MockFlow(() => this.pingActivity())
  }

  async start(): Promise<this> {
    await this.plugin.onStart(this.mock)
    await this.plugin.onReady(this.mock)
    return this
  }

  async stop(): Promise<void> {
    await this.plugin.onStop()
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
    await this.plugin.onSignal(this.mock, signal)
    return this
  }

  async injectPayload(payload: Payload): Promise<this> {
    this.pingActivity()
    await this.plugin.onPayload(this.mock, payload)
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
