import * as grpc from '@grpc/grpc-js'
import type {
  handleUnaryCall,
  handleBidiStreamingCall,
  ServerWritableStream,
} from '@grpc/grpc-js'
import {
  type PingRequest,
  type PingResponse,
  type ListPluginsRequest,
  type ListPluginsResponse,
  type CreateInstanceRequest,
  type CreateInstanceResponse,
  type DestroyInstanceRequest,
  type DestroyInstanceResponse,
  type RuntimeMessage,
  type RemotePluginServiceServer,
} from '@/proto/plugin.js'
import { pluginMetadataToProto } from '@/internal/converters.js'
import { PluginInstanceService } from '@/internal/instances.js'
import { PluginSession } from '@/internal/session.js'
import { Channel } from '@/internal/channel.js'
import { createLogMessageBuilder } from '@/internal/logger.js'
import type { Config } from '@/app.js'

// ---------------------------------------------------------------------------
// RemotePluginServiceHandler
// ---------------------------------------------------------------------------

/**
 * gRPC service implementation for RemotePluginService.
 *
 * Handles all RPC methods: Ping, ListPlugins, CreateInstance,
 * DestroyInstance, and the bidirectional RunInstance stream.
 */
export const createServiceHandler = (
  pluginInstances: PluginInstanceService,
  config: Config,
): RemotePluginServiceServer => {
  const serverId = config.serverId
  const version = config.version

  // ── Ping ─────────────────────────────────────────────────────────────────

  const ping: handleUnaryCall<PingRequest, PingResponse> = (call, callback) => {
    console.debug(`remote plugin ping client_id=${call.request.clientId}`)
    callback(null, { serverId, version })
  }

  // ── ListPlugins ──────────────────────────────────────────────────────────

  const listPlugins: handleUnaryCall<ListPluginsRequest, ListPluginsResponse> = (
    call,
    callback,
  ) => {
    const plugins = pluginInstances.listMetadata().map(pluginMetadataToProto)
    console.info(
      `remote plugin list plugins namespace=${call.request.namespace} plugin_count=${plugins.length}`,
    )
    callback(null, { plugins })
  }

  // ── CreateInstance ────────────────────────────────────────────────────────

  const createInstance: handleUnaryCall<CreateInstanceRequest, CreateInstanceResponse> = (
    call,
    callback,
  ) => {
    try {
      pluginInstances.createInstance(
        call.request.instanceId,
        call.request.pluginName,
        call.request.config,
      )
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      if (message.includes('not found')) {
        return callback({
          code: grpc.status.NOT_FOUND,
          message,
        })
      }
      return callback({
        code: grpc.status.INVALID_ARGUMENT,
        message,
      })
    }
    console.info(
      `remote plugin instance created instance_id=${call.request.instanceId} plugin=${call.request.pluginName}`,
    )
    callback(null, { instanceId: call.request.instanceId })
  }

  // ── DestroyInstance ──────────────────────────────────────────────────────

  const destroyInstance: handleUnaryCall<DestroyInstanceRequest, DestroyInstanceResponse> = (
    call,
    callback,
  ) => {
    pluginInstances
      .destroyInstance(call.request.instanceId)
      .then(() => {
        console.info(
          `remote plugin instance destroyed instance_id=${call.request.instanceId} reason=${call.request.reason}`,
        )
        callback(null, {})
      })
      .catch((err) => {
        callback({
          code: grpc.status.INTERNAL,
          message: err instanceof Error ? err.message : String(err),
        })
      })
  }

  // ── RunInstance (bidi stream) ─────────────────────────────────────────────

  const runInstance: handleBidiStreamingCall<RuntimeMessage, RuntimeMessage> = (call) => {
    const metadata = call.metadata.getMap()
    const instanceId = metadata['instance-id'] as string | undefined

    if (!instanceId) {
      call.destroy(
        new Error('instance-id metadata is required'),
      )
      return
    }

    let handlers, logHandler
    try {
      ;[handlers, logHandler] = pluginInstances.getInstanceWithHandler(instanceId)
    } catch (err) {
      call.destroy(
        new Error(err instanceof Error ? err.message : String(err)),
      )
      return
    }

    const outgoingMessages = new Channel<RuntimeMessage>()
    const ac = new AbortController()

    // Start log forwarding
    const logMsgBuilder = createLogMessageBuilder(instanceId)
    const logPromise = logHandler.readLoop(outgoingMessages, logMsgBuilder, ac.signal)

    // Create session
    const session = new PluginSession(instanceId, handlers, outgoingMessages, {
      ackIntervalSec: config.ackIntervalSec,
    })

    try {
      pluginInstances.attachSession(instanceId, session)
    } catch (err) {
      call.destroy(
        new Error(err instanceof Error ? err.message : String(err)),
      )
      return
    }

    // Process incoming stream (runs in background)
    const streamAsIterable = makeCallIterable(call)
    const processPromise = session.processStream(streamAsIterable)

    // Consume outgoing queue → write to gRPC stream
    const writeLoop = (async () => {
      while (true) {
        const msg = await outgoingMessages.get()
        if (msg === null) break
        if (!call.writable) break
        call.write(msg)
      }
    })()

    // Cleanup on stream end
    const cleanup = async () => {
      await session.close()
      pluginInstances.detachSession(instanceId, session)
      ac.abort()
      await Promise.allSettled([processPromise, logPromise, writeLoop])
      if (call.writable) call.end()
    }

    call.on('end', () => {
      cleanup().catch((err) => {
        console.error(`remote plugin cleanup failed instance_id=${instanceId}`, err)
      })
    })

    call.on('error', (err) => {
      if ((err as NodeJS.ErrnoException).code !== 'ERR_STREAM_WRITE_AFTER_END') {
        console.error(`remote plugin stream error instance_id=${instanceId}`, err)
      }
      cleanup().catch(() => {})
    })

    call.on('cancelled', () => {
      console.info(`remote plugin stream cancelled instance_id=${instanceId}`)
      cleanup().catch(() => {})
    })
  }

  return {
    ping,
    listPlugins,
    createInstance,
    destroyInstance,
    runInstance,
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Wrap a gRPC bidi call's readable side as an AsyncIterable.
 */
function makeCallIterable(
  call: grpc.ServerDuplexStream<RuntimeMessage, RuntimeMessage>,
): AsyncIterable<RuntimeMessage> {
  return {
    [Symbol.asyncIterator]() {
      const buffer: RuntimeMessage[] = []
      let resolve: ((value: IteratorResult<RuntimeMessage>) => void) | null = null
      let done = false

      call.on('data', (msg: RuntimeMessage) => {
        if (resolve) {
          const r = resolve
          resolve = null
          r({ value: msg, done: false })
        } else {
          buffer.push(msg)
        }
      })

      const finish = () => {
        done = true
        if (resolve) {
          const r = resolve
          resolve = null
          r({ value: undefined as unknown as RuntimeMessage, done: true })
        }
      }

      call.on('end', finish)
      call.on('error', finish)
      call.on('cancelled', finish)

      return {
        next(): Promise<IteratorResult<RuntimeMessage>> {
          if (buffer.length > 0) {
            return Promise.resolve({ value: buffer.shift()!, done: false })
          }
          if (done) {
            return Promise.resolve({
              value: undefined as unknown as RuntimeMessage,
              done: true,
            })
          }
          return new Promise<IteratorResult<RuntimeMessage>>((r) => {
            resolve = r
          })
        },
      }
    },
  }
}
