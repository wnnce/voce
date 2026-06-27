import type { z, ZodType } from 'zod'
import { zodToJsonSchema } from 'zod-to-json-schema'
import type {
  AsyncPlugin,
  MultiTrackConfig,
  PluginMetadata,
  PortMetadata,
  PropertyDef,
} from './plugin.js'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Constructor type for an AsyncPlugin subclass. */
export type AsyncPluginClass<TConfig = unknown> = new (config: TConfig) => AsyncPlugin<TConfig>

// ---------------------------------------------------------------------------
// PluginRegistry
// ---------------------------------------------------------------------------

export class PluginRegistry {
  private readonly plugins = new Map<string, AsyncPluginClass<unknown>>()
  private readonly schemas = new Map<string, ZodType | undefined>()
  private readonly metadata = new Map<string, PluginMetadata>()

  register<TConfig>(
    meta: PluginMetadata,
    pluginCls: AsyncPluginClass<TConfig>,
    configSchema?: ZodType,
  ): void {
    if (this.plugins.has(meta.name)) {
      throw new Error(`plugin already registered: ${meta.name}`)
    }
    this.plugins.set(meta.name, pluginCls as AsyncPluginClass<unknown>)
    this.schemas.set(meta.name, configSchema)
    this.metadata.set(meta.name, this.metadataWithSchema(meta, configSchema))
  }

  listMetadata(): PluginMetadata[] {
    return [...this.metadata.values()]
  }

  create(name: string, configBytes: Buffer | Uint8Array): AsyncPlugin<unknown> {
    const pluginCls = this.plugins.get(name)
    if (!pluginCls) throw new Error(`plugin not found: ${name}`)

    const schema = this.schemas.get(name)
    const config = this.decodeConfig(schema, configBytes)
    return new pluginCls(config)
  }

  // ── private ──────────────────────────────────────────────────────────────

  private decodeConfig(schema: ZodType | undefined, raw: Buffer | Uint8Array): unknown {
    if (!raw || raw.length === 0) {
      return schema ? schema.parse({}) : {}
    }
    const parsed: unknown = JSON.parse(Buffer.from(raw).toString('utf-8'))
    return schema ? schema.parse(parsed) : parsed
  }

  private metadataWithSchema(meta: PluginMetadata, schema?: ZodType): PluginMetadata {
    if (meta.schema != null || !schema) return meta
    return { ...meta, schema: zodToJsonSchema(schema) as Record<string, unknown> }
  }
}

/** Global singleton registry. */
export const pluginRegistry = new PluginRegistry()

// ---------------------------------------------------------------------------
// @plugin decorator
// ---------------------------------------------------------------------------

export interface PluginOptions {
  name: string
  description?: string
  configSchema?: ZodType
  inputs?: PropertyDef[]
  outputs?: PropertyDef[]
  ports?: PortMetadata[]
  multiTrack?: MultiTrackConfig
}

/**
 * Class decorator that registers a plugin in the global registry.
 *
 * @example
 * ```ts
 * const ConfigSchema = z.object({ threshold: z.number().default(0.5) })
 * type Config = z.infer<typeof ConfigSchema>
 *
 * @plugin({ name: 'my-plugin', configSchema: ConfigSchema })
 * class MyPlugin extends AsyncPlugin<Config> {
 *   async onPayload(flow: Flow, payload: Payload) {
 *     await flow.sendPayload(payload)
 *   }
 * }
 * ```
 */
export function plugin<TConfig = unknown>(options: PluginOptions) {
  return function decorator<T extends AsyncPluginClass<TConfig>>(target: T): T {
    const meta: PluginMetadata = {
      name: options.name,
      description: options.description ?? '',
      inputs: options.inputs ?? [],
      outputs: options.outputs ?? [],
      ports: options.ports ?? [],
      multiTrack: options.multiTrack,
    }
    pluginRegistry.register(meta, target as AsyncPluginClass<unknown>, options.configSchema)
    return target
  }
}

/**
 * Functional (non-decorator) API for registering a plugin.
 *
 * Useful when decorators are not desired or when configSchema type inference
 * needs to be explicit.
 */
export function registerPlugin<TConfig>(
  pluginCls: AsyncPluginClass<TConfig>,
  options: PluginOptions,
): void {
  const meta: PluginMetadata = {
    name: options.name,
    description: options.description ?? '',
    inputs: options.inputs ?? [],
    outputs: options.outputs ?? [],
    ports: options.ports ?? [],
    multiTrack: options.multiTrack,
  }
  pluginRegistry.register(meta, pluginCls as AsyncPluginClass<unknown>, options.configSchema)
}
