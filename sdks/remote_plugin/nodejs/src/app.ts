import * as grpc from '@grpc/grpc-js'
import { pluginRegistry } from './core/registry.js'
import { PluginInstanceService } from './service/instances.js'
import { createServiceHandler } from './service/handler.js'
import { RemotePluginServiceService } from './proto/plugin.js'

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

export interface Config {
  host: string
  port: number
  serverId: string
  version: string
  /** Grace period in milliseconds when stopping the server. */
  stopGracePeriodMs: number
  /** Interval in seconds between ACK keepalive messages. */
  ackIntervalSec: number
  /** Maximum number of buffered log messages per plugin instance. */
  logQueueMaxSize: number
  /** Minimum log level forwarded through gRPC: 'debug' | 'info' | 'warn' | 'error'. */
  logLevel: string
}

const DEFAULT_CONFIG: Config = {
  host: '127.0.0.1',
  port: 50051,
  serverId: 'nodejs-remote-plugin',
  version: '0.1.0',
  stopGracePeriodMs: 5000,
  ackIntervalSec: 10,
  logQueueMaxSize: 1000,
  logLevel: 'info',
}

/** Merge user-supplied partial config with defaults. */
export function resolveConfig(partial?: Partial<Config>): Config {
  return { ...DEFAULT_CONFIG, ...partial }
}

// ---------------------------------------------------------------------------
// App
// ---------------------------------------------------------------------------

/**
 * Voce remote plugin application.
 *
 * Provides lifecycle management (start, stop, serve) for the
 * underlying gRPC server.
 */
export class App {
  readonly config: Config
  private server: grpc.Server | null = null
  private readonly pluginInstances: PluginInstanceService

  constructor(config?: Partial<Config>) {
    this.config = resolveConfig(config)
    this.pluginInstances = new PluginInstanceService(pluginRegistry, this.config)
  }

  /** Start the gRPC server without blocking. */
  async start(): Promise<void> {
    if (this.server) {
      throw new Error('App is already started')
    }

    this.server = new grpc.Server()
    const handler = createServiceHandler(this.pluginInstances, this.config)
    this.server.addService(RemotePluginServiceService, handler)

    const listenAddr = `${this.config.host}:${this.config.port}`

    const boundPort = await new Promise<number>((resolve, reject) => {
      this.server!.bindAsync(
        listenAddr,
        grpc.ServerCredentials.createInsecure(),
        (err, port) => {
          if (err) return reject(err)
          resolve(port)
        },
      )
    })

    if (boundPort === 0) {
      throw new Error(`failed to bind remote plugin server: ${listenAddr}`)
    }

    console.info(
      `remote plugin server starting server_id=${this.config.serverId} ` +
        `version=${this.config.version} listen_addr=${listenAddr} ` +
        `plugin_count=${pluginRegistry.listMetadata().length}`,
    )

    console.info(`remote plugin server started listen_addr=${listenAddr}`)
  }

  /** Gracefully stop the gRPC server. */
  async stop(graceMs?: number): Promise<void> {
    if (!this.server) return

    const grace = graceMs ?? this.config.stopGracePeriodMs
    const listenAddr = `${this.config.host}:${this.config.port}`
    console.info(`remote plugin server stopping listen_addr=${listenAddr}`)

    await new Promise<void>((resolve) => {
      const timer = setTimeout(() => {
        console.warn(`remote plugin server force shutdown listen_addr=${listenAddr}`)
        this.server!.forceShutdown()
        resolve()
      }, grace)

      this.server!.tryShutdown((err) => {
        clearTimeout(timer)
        if (err) {
          console.error(`remote plugin server shutdown error`, err)
        }
        resolve()
      })
    })

    console.info(`remote plugin server stopped listen_addr=${listenAddr}`)
    this.server = null
  }

  /**
   * Start the server, block until a shutdown signal is received,
   * then stop gracefully.
   */
  async serve(): Promise<void> {
    await this.start()

    await new Promise<void>((resolve) => {
      const shutdown = (signal: string) => {
        console.info(`remote plugin server received shutdown signal signal=${signal}`)
        resolve()
      }

      process.once('SIGINT', () => shutdown('SIGINT'))
      process.once('SIGTERM', () => shutdown('SIGTERM'))
    })

    await this.stop()
  }
}
