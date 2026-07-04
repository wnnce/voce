import { describe, it, expect, beforeEach } from 'vitest'
import { z } from 'zod'
import { PluginRegistry, definePlugin, type PluginContext } from '@/plugin.js'

// ── Test fixtures ──────────────────────────────────────────────────────────

const TestConfigSchema = z.object({
  threshold: z.number().default(0.5),
  label: z.string().default('test'),
})

const testPlugin = definePlugin({
  name: 'test-plugin',
  description: 'A test plugin',
  configSchema: TestConfigSchema,
  setup(context) {
    return {
      async onStart() {
        context.logger.info('started')
      },
    }
  },
})

const noopContext: Omit<PluginContext, 'config'> = {
  instanceId: 'test-1',
  logger: {
    debug: () => { },
    info: () => { },
    warn: () => { },
    error: () => { },
  },
}

// ── Tests ──────────────────────────────────────────────────────────────────

describe('PluginRegistry', () => {
  let registry: PluginRegistry

  beforeEach(() => {
    registry = new PluginRegistry()
  })

  it('should register and list plugin metadata', () => {
    registry.register(testPlugin)
    const all = registry.listMetadata()
    expect(all).toHaveLength(1)
    expect(all[0].name).toBe('test-plugin')
  })

  it('should auto-inject JSON schema from zod', () => {
    registry.register(testPlugin)
    const meta = registry.listMetadata()[0]
    expect(meta.schema).toBeDefined()
    expect((meta.schema as Record<string, unknown>).type).toBe('object')
  })

  it('should throw on duplicate registration', () => {
    registry.register(testPlugin)
    expect(() => {
      registry.register(testPlugin)
    }).toThrow('plugin already registered: test-plugin')
  })

  it('should create a plugin instance with config validation', async () => {
    registry.register(testPlugin)
    const config = Buffer.from(JSON.stringify({ threshold: 0.8 }))

    // Test that the handler is created successfully.
    // In actual implementation, setup receives the merged config.
    // We can test this by mutating the context or throwing if it's wrong,
    // but here we just check it returns the handlers.
    const handlers = await registry.createInstance('test-plugin', noopContext, config)
    expect(handlers.onStart).toBeDefined()
  })

  it('should create a plugin instance with empty config (using defaults)', async () => {
    registry.register(testPlugin)
    const handlers = await registry.createInstance('test-plugin', noopContext, Buffer.alloc(0))
    expect(handlers.onStart).toBeDefined()
  })

  it('should throw on invalid config', async () => {
    registry.register(testPlugin)
    const config = Buffer.from(JSON.stringify({ threshold: 'not-a-number' }))
    await expect(registry.createInstance('test-plugin', noopContext, config)).rejects.toThrow()
  })

  it('should throw on unknown plugin', async () => {
    await expect(registry.createInstance('nonexistent', noopContext, Buffer.alloc(0))).rejects.toThrow(
      'plugin not found: nonexistent',
    )
  })
})
