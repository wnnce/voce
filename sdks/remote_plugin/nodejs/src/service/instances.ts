import type { AsyncPlugin } from '../core/plugin.js'
import type { PluginRegistry } from '../core/registry.js'
import { LogLevel } from '../proto/plugin.js'
import { RemoteLogHandler, createPluginLogger } from '../core/logger.js'
import type { PluginSession } from './session.js'
import type { Config } from '../app.js'

// ---------------------------------------------------------------------------
// PluginInstance
// ---------------------------------------------------------------------------

interface PluginInstance {
  plugin: AsyncPlugin<unknown>
  logHandler: RemoteLogHandler
  session: PluginSession | null
}

// ---------------------------------------------------------------------------
// PluginInstanceService
// ---------------------------------------------------------------------------

/**
 * Manages the lifecycle of plugin instances: creation, destruction,
 * session attachment, and log handler wiring.
 */
export class PluginInstanceService {
  private readonly registry: PluginRegistry
  private readonly config: Config
  private readonly instances = new Map<string, PluginInstance>()

  constructor(registry: PluginRegistry, config: Config) {
    this.registry = registry
    this.config = config
  }

  createInstance(instanceId: string, pluginName: string, configBytes: Buffer | Uint8Array): void {
    if (!instanceId) throw new Error('instance_id is required')
    if (this.instances.has(instanceId)) {
      throw new Error(`plugin instance already exists: ${instanceId}`)
    }

    const plugin = this.registry.create(pluginName, configBytes)

    const logHandler = new RemoteLogHandler(this.config.logQueueMaxSize)
    const logLevel = mapLogLevelConfig(this.config.logLevel)
    plugin.logger = createPluginLogger(logHandler, logLevel)

    this.instances.set(instanceId, { plugin, logHandler, session: null })
  }

  async destroyInstance(instanceId: string): Promise<void> {
    const instance = this.instances.get(instanceId)
    if (!instance) return

    try {
      if (instance.session) {
        await instance.session.close()
      }
      instance.logHandler.close()
    } finally {
      this.instances.delete(instanceId)
    }
  }

  getInstance(instanceId: string): AsyncPlugin<unknown> {
    const instance = this.instances.get(instanceId)
    if (!instance) throw new Error(`plugin instance not found: ${instanceId}`)
    return instance.plugin
  }

  getInstanceWithHandler(
    instanceId: string,
  ): [AsyncPlugin<unknown>, RemoteLogHandler] {
    const instance = this.instances.get(instanceId)
    if (!instance) throw new Error(`plugin instance not found: ${instanceId}`)
    return [instance.plugin, instance.logHandler]
  }

  attachSession(instanceId: string, session: PluginSession): void {
    const instance = this.instances.get(instanceId)
    if (!instance) throw new Error(`plugin instance not found: ${instanceId}`)
    if (instance.session && instance.session !== session) {
      throw new Error(`plugin instance already has an active stream: ${instanceId}`)
    }
    instance.session = session
  }

  detachSession(instanceId: string, session: PluginSession): void {
    const instance = this.instances.get(instanceId)
    if (instance && instance.session === session) {
      instance.session = null
    }
  }

  listMetadata() {
    return this.registry.listMetadata()
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function mapLogLevelConfig(level: string): LogLevel {
  switch (level.toLowerCase()) {
    case 'debug':
      return LogLevel.LOG_LEVEL_DEBUG
    case 'info':
      return LogLevel.LOG_LEVEL_INFO
    case 'warn':
    case 'warning':
      return LogLevel.LOG_LEVEL_WARN
    case 'error':
      return LogLevel.LOG_LEVEL_ERROR
    default:
      return LogLevel.LOG_LEVEL_INFO
  }
}
