import type { PluginRegistry } from '@/plugin.js'
import { LogLevel } from '@/proto/plugin.js'
import { RemoteLogHandler, createPluginLogger } from '@/internal/logger.js'
import type { PluginSession } from '@/internal/session.js'
import type { Config } from '@/app.js'
import type { PluginHandlers } from '@/plugin.js'

// ---------------------------------------------------------------------------
// PluginInstance
// ---------------------------------------------------------------------------

interface PluginInstance {
  handlers: PluginHandlers
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

  async createInstance(instanceId: string, pluginName: string, configBytes: Buffer | Uint8Array): Promise<void> {
    if (!instanceId) throw new Error('instance_id is required')
    if (this.instances.has(instanceId)) {
      throw new Error(`plugin instance already exists: ${instanceId}`)
    }

    const logHandler = new RemoteLogHandler(this.config.logQueueMaxSize)
    const logLevel = mapLogLevelConfig(this.config.logLevel)
    const logger = createPluginLogger(logHandler, logLevel)

    // Execute setup() to get the handlers
    const handlers = await this.registry.createInstance(
      pluginName,
      { instanceId, logger },
      configBytes
    )

    this.instances.set(instanceId, { handlers, logHandler, session: null })
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

  getHandlers(instanceId: string): PluginHandlers {
    const instance = this.instances.get(instanceId)
    if (!instance) throw new Error(`plugin instance not found: ${instanceId}`)
    return instance.handlers
  }

  getInstanceWithHandler(
    instanceId: string,
  ): [PluginHandlers, RemoteLogHandler] {
    const instance = this.instances.get(instanceId)
    if (!instance) throw new Error(`plugin instance not found: ${instanceId}`)
    return [instance.handlers, instance.logHandler]
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
