import { describe, it, expect, beforeEach } from 'vitest'
import { z } from 'zod'
import { PluginRegistry, type AsyncPluginClass } from '../src/core/registry.js'
import { AsyncPlugin, type PluginMetadata } from '../src/core/plugin.js'

// ── Test fixtures ──────────────────────────────────────────────────────────

const TestConfigSchema = z.object({
  threshold: z.number().default(0.5),
  label: z.string().default('test'),
})
type TestConfig = z.infer<typeof TestConfigSchema>

class TestPlugin extends AsyncPlugin<TestConfig> {}

const testMeta: PluginMetadata = {
  name: 'test-plugin',
  description: 'A test plugin',
}

// ── Tests ──────────────────────────────────────────────────────────────────

describe('PluginRegistry', () => {
  let registry: PluginRegistry

  beforeEach(() => {
    registry = new PluginRegistry()
  })

  it('should register and list plugin metadata', () => {
    registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    const all = registry.listMetadata()
    expect(all).toHaveLength(1)
    expect(all[0].name).toBe('test-plugin')
  })

  it('should auto-inject JSON schema from zod', () => {
    registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    const meta = registry.listMetadata()[0]
    expect(meta.schema).toBeDefined()
    expect((meta.schema as Record<string, unknown>).type).toBe('object')
  })

  it('should not override user-provided schema', () => {
    const customSchema = { custom: true }
    const metaWithSchema: PluginMetadata = { ...testMeta, schema: customSchema }
    registry.register(metaWithSchema, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    expect(registry.listMetadata()[0].schema).toEqual(customSchema)
  })

  it('should throw on duplicate registration', () => {
    registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    expect(() => {
      registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    }).toThrow('plugin already registered: test-plugin')
  })

  it('should create a plugin instance with config validation', () => {
    registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    const config = Buffer.from(JSON.stringify({ threshold: 0.8 }))
    const instance = registry.create('test-plugin', config)
    expect(instance).toBeInstanceOf(TestPlugin)
    expect((instance.config as TestConfig).threshold).toBe(0.8)
    expect((instance.config as TestConfig).label).toBe('test') // default
  })

  it('should create a plugin instance with empty config', () => {
    registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    const instance = registry.create('test-plugin', Buffer.alloc(0))
    expect((instance.config as TestConfig).threshold).toBe(0.5) // default
  })

  it('should throw on invalid config', () => {
    registry.register(testMeta, TestPlugin as AsyncPluginClass<unknown>, TestConfigSchema)
    const config = Buffer.from(JSON.stringify({ threshold: 'not-a-number' }))
    expect(() => registry.create('test-plugin', config)).toThrow()
  })

  it('should throw on unknown plugin', () => {
    expect(() => registry.create('nonexistent', Buffer.alloc(0))).toThrow(
      'plugin not found: nonexistent',
    )
  })
})
