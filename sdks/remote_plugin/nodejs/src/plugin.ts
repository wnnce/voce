import type { z, ZodType } from 'zod'
import { zodToJsonSchema } from 'zod-to-json-schema'
import type { Signal, Payload } from '@/types.js'

// ---------------------------------------------------------------------------
// Flow
// ---------------------------------------------------------------------------

/**
 * Flow interface used by plugins to emit signals and payloads back into the DAG.
 */
export interface Flow {
  sendPayload(payload: Payload, options?: { port?: number }): Promise<void>
  sendSignal(signal: Signal, options?: { port?: number }): Promise<void>
}

// ---------------------------------------------------------------------------
// Metadata types
// ---------------------------------------------------------------------------

export interface FieldDef {
  key: string
  type: 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array'
  required?: boolean
}

export interface PropertyDef {
  type: 'signal' | 'payload' | 'audio' | 'video'
  name?: string
  fields?: FieldDef[]
}

export interface PortMetadata {
  type: 'signal' | 'payload' | 'audio' | 'video'
  port: number
  name?: string
  description?: string
}

export interface TrackConfig {
  enabled?: boolean
  bufferSize?: number
  dropStrategy?: 'block_if_full' | 'drop_newest' | 'drop_oldest'
  interruptSignals?: string[]
}

export interface MultiTrackConfig {
  enabled?: boolean
  payload?: TrackConfig
}

export interface PluginMetadata {
  name: string
  description?: string
  schema?: Record<string, unknown>
  inputs?: PropertyDef[]
  outputs?: PropertyDef[]
  ports?: PortMetadata[]
  multiTrack?: MultiTrackConfig
}

// ---------------------------------------------------------------------------
// Logger interface
// ---------------------------------------------------------------------------

export interface PluginLogger {
  debug(message: string, ...args: unknown[]): void
  info(message: string, ...args: unknown[]): void
  warn(message: string, ...args: unknown[]): void
  error(message: string, ...args: unknown[]): void
}

// ---------------------------------------------------------------------------
// Plugin Definition & Hooks
// ---------------------------------------------------------------------------

/**
 * Context provided to the plugin during initialization (setup).
 * Includes the typed configuration, logger, and instanceId.
 */
export interface PluginContext<TConfig = unknown> {
  instanceId: string
  config: TConfig
  logger: PluginLogger
}

/**
 * Handlers returned by the plugin setup function.
 * Called during the plugin's runtime lifecycle.
 */
export interface PluginHandlers {
  onStart?(flow: Flow): Promise<void> | void
  onReady?(flow: Flow): Promise<void> | void
  onPause?(): Promise<void> | void
  onResume?(flow: Flow): Promise<void> | void
  onStop?(): Promise<void> | void
  onSignal?(signal: Signal, flow: Flow): Promise<void> | void
  onPayload?(payload: Payload, flow: Flow): Promise<void> | void
}

/**
 * Options used to define a plugin.
 */
export interface PluginOptions<TConfig = unknown> {
  name: string
  description?: string
  configSchema?: ZodType<TConfig>
  inputs?: PropertyDef[]
  outputs?: PropertyDef[]
  ports?: PortMetadata[]
  multiTrack?: MultiTrackConfig
  /**
   * Factory function called when a new instance of the plugin is created.
   * Receives the runtime context and should return the event handlers.
   */
  setup: (context: PluginContext<TConfig>) => PluginHandlers | Promise<PluginHandlers>
}

/**
 * The defined plugin object, containing metadata and the factory.
 */
export interface PluginDefinition<TConfig = unknown> {
  metadata: PluginMetadata
  configSchema?: ZodType<TConfig>
  setup: (context: PluginContext<TConfig>) => PluginHandlers | Promise<PluginHandlers>
}

/**
 * Creates a plugin definition object.
 *
 * @example
 * ```ts
 * const MyConfigSchema = z.object({ threshold: z.number().default(0.5) })
 *
 * export const myPlugin = definePlugin({
 *   name: 'my-plugin',
 *   configSchema: MyConfigSchema,
 *   setup({ config, logger }) {
 *     return {
 *       async onPayload(payload, flow) {
 *         logger.info(`Received payload: ${payload.name}`)
 *         await flow.sendPayload(payload)
 *       }
 *     }
 *   }
 * })
 * ```
 */
export const definePlugin = <TConfig = unknown>(
  options: PluginOptions<TConfig>,
): PluginDefinition<TConfig> => {
  return {
    metadata: {
      name: options.name,
      description: options.description ?? '',
      inputs: options.inputs ?? [],
      outputs: options.outputs ?? [],
      ports: options.ports ?? [],
      multiTrack: options.multiTrack,
    },
    configSchema: options.configSchema,
    setup: options.setup,
  }
}

// ---------------------------------------------------------------------------
// PluginRegistry
// ---------------------------------------------------------------------------

export class PluginRegistry {
  private readonly plugins = new Map<string, PluginDefinition<unknown>>()
  private readonly metadata = new Map<string, PluginMetadata>()

  /**
   * Register a plugin definition.
   */
  register<TConfig>(definition: PluginDefinition<TConfig>): void {
    const meta = definition.metadata
    if (this.plugins.has(meta.name)) {
      throw new Error(`plugin already registered: ${meta.name}`)
    }
    this.plugins.set(meta.name, definition as PluginDefinition<unknown>)
    this.metadata.set(meta.name, this.metadataWithSchema(meta, definition.configSchema))
  }

  /**
   * List all registered plugin metadata.
   */
  listMetadata(): PluginMetadata[] {
    return [...this.metadata.values()]
  }

  /**
   * Create a plugin instance by running its setup function.
   * Parses the raw config bytes using the plugin's configSchema.
   */
  async createInstance(
    name: string,
    contextWithoutConfig: Omit<PluginContext, 'config'>,
    configBytes: Buffer | Uint8Array,
  ): Promise<PluginHandlers> {
    const definition = this.plugins.get(name)
    if (!definition) throw new Error(`plugin not found: ${name}`)

    const config = this.decodeConfig(definition.configSchema, configBytes)
    const context: PluginContext = { ...contextWithoutConfig, config }

    return await definition.setup(context)
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
