import type { Flow } from './flow.js'
import type { Signal } from '../schema/signal.js'
import type { Payload } from '../schema/payload.js'

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
// Logger interface (injected by the SDK at instance creation time)
// ---------------------------------------------------------------------------

export interface PluginLogger {
  debug(message: string, ...args: unknown[]): void
  info(message: string, ...args: unknown[]): void
  warn(message: string, ...args: unknown[]): void
  error(message: string, ...args: unknown[]): void
}

// ---------------------------------------------------------------------------
// AsyncPlugin base class
// ---------------------------------------------------------------------------

/**
 * Base class for remote plugins.
 *
 * Subclasses override lifecycle and event callbacks.
 * The first remote plugin version intentionally does not expose audio or video callbacks.
 *
 * @typeParam TConfig - Plugin configuration type, validated at instance creation.
 */
export abstract class AsyncPlugin<TConfig = unknown> {
  metadata!: PluginMetadata
  logger!: PluginLogger
  config: TConfig

  constructor(config: TConfig) {
    this.config = config
  }

  // Lifecycle callbacks ─────────────────────────────────────────────────────

  onStart(_flow: Flow): Promise<void> | void {}
  onReady(_flow: Flow): Promise<void> | void {}
  onPause(): Promise<void> | void {}
  onResume(_flow: Flow): Promise<void> | void {}
  onStop(): Promise<void> | void {}

  // Event callbacks ─────────────────────────────────────────────────────────

  onSignal(_flow: Flow, _signal: Signal): Promise<void> | void {}
  onPayload(_flow: Flow, _payload: Payload): Promise<void> | void {}
}
