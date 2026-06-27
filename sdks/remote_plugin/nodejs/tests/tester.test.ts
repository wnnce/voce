import { describe, it, expect } from 'vitest'
import { z } from 'zod'
import { AsyncPlugin } from '../src/core/plugin.js'
import type { Flow } from '../src/core/flow.js'
import { PluginTester } from '../src/core/tester.js'
import { Payload } from '../src/schema/payload.js'
import { Signal } from '../src/schema/signal.js'

// ── Test fixtures ──────────────────────────────────────────────────────────

const ConfigSchema = z.object({
  prefix: z.string().default(''),
  forwardSignals: z.boolean().default(true),
})
type Config = z.infer<typeof ConfigSchema>

class PassthroughPlugin extends AsyncPlugin<Config> {
  started = false
  stopped = false

  override async onStart(_flow: Flow) {
    this.started = true
  }

  override async onStop() {
    this.stopped = true
  }

  override async onSignal(flow: Flow, signal: Signal) {
    if (!this.config.forwardSignals) return
    await flow.sendSignal(signal)
  }

  override async onPayload(flow: Flow, payload: Payload) {
    const name = `${this.config.prefix}${payload.name}`
    await flow.sendPayload(new Payload(name, payload.toDict()))
  }
}

// ── Tests ──────────────────────────────────────────────────────────────────

describe('PluginTester', () => {
  it('should start and stop the plugin', async () => {
    const plugin = new PassthroughPlugin({ prefix: '', forwardSignals: true })
    const tester = new PluginTester(plugin)
    await tester.start()
    expect(plugin.started).toBe(true)

    await tester.stop()
    expect(plugin.stopped).toBe(true)
  })

  it('should forward payloads through the plugin', async () => {
    const plugin = new PassthroughPlugin({ prefix: 'out_', forwardSignals: true })
    const received: Array<{ port: number; payload: Payload }> = []

    const tester = new PluginTester(plugin)
    tester.onPayload((port, payload) => {
      received.push({ port, payload })
    })

    await tester.start()
    await tester.injectPayload(new Payload('test', { key: 'value' }))
    await tester.wait()

    expect(received).toHaveLength(1)
    expect(received[0].payload.name).toBe('out_test')
    expect(received[0].payload.get('key')).toBe('value')
    expect(received[0].port).toBe(0)

    await tester.stop()
  })

  it('should forward signals through the plugin', async () => {
    const plugin = new PassthroughPlugin({ prefix: '', forwardSignals: true })
    const received: Array<{ port: number; signal: Signal }> = []

    const tester = new PluginTester(plugin)
    tester.onSignal((port, signal) => {
      received.push({ port, signal })
    })

    await tester.start()
    await tester.injectSignal(new Signal('interrupter', { ts: 100 }))
    await tester.wait()

    expect(received).toHaveLength(1)
    expect(received[0].signal.name).toBe('interrupter')
    expect(received[0].signal.get('ts')).toBe(100)

    await tester.stop()
  })

  it('should respect forwardSignals=false', async () => {
    const plugin = new PassthroughPlugin({ prefix: '', forwardSignals: false })
    const received: Signal[] = []

    const tester = new PluginTester(plugin)
    tester.onSignal((_port, signal) => received.push(signal))

    await tester.start()
    await tester.injectSignal(new Signal('interrupter'))
    await tester.wait()

    expect(received).toHaveLength(0)

    await tester.stop()
  })
})
