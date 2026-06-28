import { describe, it, expect, beforeAll, afterAll } from 'vitest'
import * as grpc from '@grpc/grpc-js'
import * as protoLoader from '@grpc/proto-loader'
import path from 'path'
import { App, resolveConfig } from '@/app.js'
import { pluginRegistry, definePlugin } from '@/plugin.js'

// Load the protobuf file dynamically to create a client
const PROTO_PATH = path.resolve(__dirname, '../../../../api/plugin/v1/plugin.proto')
const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: false,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
})
const protoDescriptor = grpc.loadPackageDefinition(packageDefinition) as any
const RemotePluginServiceClient = protoDescriptor.plugin.v1.RemotePluginService

describe('App & Config', () => {
  describe('resolveConfig', () => {
    it('should merge defaults with partial config', () => {
      const config = resolveConfig({ port: 12345, serverId: 'custom-id' })
      expect(config.port).toBe(12345)
      expect(config.serverId).toBe('custom-id')
      expect(config.host).toBe('127.0.0.1') // From default
    })
  })

  describe('Server lifecycle', () => {
    let app: App
    let client: any
    
    beforeAll(async () => {
      // Register a dummy plugin to test listPlugins
      pluginRegistry.register(definePlugin({
        name: 'app-test-plugin',
        setup: () => ({})
      }))
      
      app = new App({
        port: 0, // Request random available port
        serverId: 'test-app-server',
        version: '1.2.3'
      })
      
      await app.start()
      
      // We need to find what port it bound to.
      // We can hack into app.server or assume the standard grpc behavior isn't easily accessible without it.
      // The `App.start()` method binds to port 0 but we didn't store the actual bound port.
      // Let's modify the test to use a specific high port since we don't return the bound port from start().
    })
    
    afterAll(async () => {
      if (client) {
        client.close()
      }
      if (app) {
        await app.stop()
      }
    })

    // To test `App` properly with a real client, we need a known port.
    // If we use port 0, we can't easily connect a client unless we expose the port.
    // So let's re-start the app on a specific high port just for this test.
    it('should respond to Ping', async () => {
      const testApp = new App({
        port: 50066,
        serverId: 'test-ping-server',
        version: '1.2.3'
      })
      await testApp.start()

      const pingClient = new RemotePluginServiceClient(
        '127.0.0.1:50066',
        grpc.credentials.createInsecure()
      )

      const res = await new Promise<any>((resolve, reject) => {
        pingClient.ping({ clientId: 'test-client' }, (err: any, response: any) => {
          if (err) reject(err)
          else resolve(response)
        })
      })

      expect(res.serverId).toBe('test-ping-server')
      expect(res.version).toBe('1.2.3')

      pingClient.close()
      await testApp.stop()
    })

    it('should throw if started twice', async () => {
      const testApp = new App({ port: 50067 })
      await testApp.start()
      await expect(testApp.start()).rejects.toThrow(/already started/)
      await testApp.stop()
    })
  })
})
