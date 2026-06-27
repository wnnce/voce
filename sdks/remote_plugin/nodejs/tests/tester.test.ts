import { describe, it, expect } from 'vitest'
import { z } from 'zod'
import { definePlugin } from '@/plugin.js'
import { PluginTester } from '@/testing.js'
import { createPayload, type Payload, createSignal, type Signal } from '@/types.js'

// ── Test fixtures ──────────────────────────────────────────────────────────

const ConfigSchema = z.object({
  prefix: z.string().default(''),
  forwardSignals: z.boolean().default(true),
})

const passthroughPlugin = definePlugin({
  name: 'passthrough',
  configSchema: ConfigSchema,
  setup({ config }) {
    let started = false
    let stopped = false

    return {
      async onStart() {
        started = true
      },
      async onStop() {
        stopped = true
      },
      async onSignal(signal, flow) {
        if (!config.forwardSignals) return
        await flow.sendSignal(signal)
      },
      async onPayload(payload, flow) {
        const name = `${config.prefix}${payload.name}`
        await flow.sendPayload(createPayload(name, payload.properties))
      },
      // Expose state for testing via an undocumented hack or just let the tester check side effects
      // Since closures hide state, we test the side effects (emitted signals/payloads) instead of
      // inspecting 'started'/'stopped', but let's leave them for completeness.
    }
  },
})

// ── Tests ──────────────────────────────────────────────────────────────────

describe('PluginTester', () => {
  it('should start and stop the plugin without errors', async () => {
    const tester = await PluginTester.create(passthroughPlugin, { prefix: '', forwardSignals: true })
    await tester.start()
    await tester.stop()
    // No easy way to read 'started' and 'stopped' from the closure, 
    // but we verify no exceptions are thrown.
  })

  it('should forward payloads through the plugin', async () => {
    const received: Array<{ port: number; payload: Payload }> = []

    const tester = await PluginTester.create(passthroughPlugin, { prefix: 'out_', forwardSignals: true })
    tester.onPayload((port, payload) => {
      received.push({ port, payload })
    })

    await tester.start()
    await tester.injectPayload(createPayload('test', { key: 'value' }))
    await tester.wait()

    expect(received).toHaveLength(1)
    expect(received[0].payload.name).toBe('out_test')
    expect(received[0].payload.properties['key']).toBe('value')
    expect(received[0].port).toBe(0)

    await tester.stop()
  })

  it('should forward signals through the plugin', async () => {
    const received: Array<{ port: number; signal: Signal }> = []

    const tester = await PluginTester.create(passthroughPlugin, { prefix: '', forwardSignals: true })
    tester.onSignal((port, signal) => {
      received.push({ port, signal })
    })

    await tester.start()
    await tester.injectSignal(createSignal('interrupter', { ts: 100 }))
    await tester.wait()

    expect(received).toHaveLength(1)
    expect(received[0].signal.name).toBe('interrupter')
    expect(received[0].signal.properties['ts']).toBe(100)

    await tester.stop()
  })

  it('should respect forwardSignals=false config', async () => {
    const received: Signal[] = []

    const tester = await PluginTester.create(passthroughPlugin, { prefix: '', forwardSignals: false })
    tester.onSignal((_port, signal) => received.push(signal))

    await tester.start()
    await tester.injectSignal(createSignal('interrupter'))
    await tester.wait()

    expect(received).toHaveLength(0)

    await tester.stop()
  })
})
