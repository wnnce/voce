import { describe, it, expect, vi, beforeEach } from 'vitest'
import { PluginInstanceService } from '@/internal/instances.js'
import { PluginRegistry, definePlugin } from '@/plugin.js'
import { PluginSession } from '@/internal/session.js'
import { Channel } from '@/internal/channel.js'
import type { Config } from '@/app.js'

describe('PluginInstanceService', () => {
  let registry: PluginRegistry
  let config: Config
  let service: PluginInstanceService

  beforeEach(() => {
    registry = new PluginRegistry()
    config = {
      host: '127.0.0.1',
      port: 50051,
      serverId: 'test',
      version: '1.0.0',
      stopGracePeriodMs: 1000,
      ackIntervalSec: 10,
      logQueueMaxSize: 100,
      logLevel: 'info',
    }
    service = new PluginInstanceService(registry, config)
  })

  describe('createInstance', () => {
    it('should create an instance successfully', async () => {
      const setupSpy = vi.fn().mockResolvedValue({ onSignal: vi.fn() })
      
      const testPlugin = definePlugin({
        name: 'test-plugin',
        setup: setupSpy,
      })
      registry.register(testPlugin)

      await service.createInstance('inst-1', 'test-plugin', Buffer.from('{}'))

      expect(setupSpy).toHaveBeenCalledOnce()
      const callArgs = setupSpy.mock.calls[0][0]
      expect(callArgs.instanceId).toBe('inst-1')
      expect(callArgs.logger).toBeDefined()
      expect(callArgs.config).toEqual({})

      // Verify it was added
      const [handlers, logHandler] = service.getInstanceWithHandler('inst-1')
      expect(handlers).toBeDefined()
      expect(logHandler).toBeDefined()
    })

    it('should throw if instance already exists', async () => {
      registry.register(definePlugin({ name: 'p1', setup: () => ({}) }))
      await service.createInstance('inst-1', 'p1', Buffer.from(''))
      
      await expect(service.createInstance('inst-1', 'p1', Buffer.from(''))).rejects.toThrow(
        /plugin instance already exists/
      )
    })

    it('should throw if plugin does not exist', async () => {
      await expect(service.createInstance('inst-1', 'unknown', Buffer.from(''))).rejects.toThrow(
        /plugin not found/
      )
    })
  })

  describe('destroyInstance', () => {
    it('should cleanly remove the instance and its resources', async () => {
      registry.register(definePlugin({ name: 'p1', setup: () => ({}) }))
      await service.createInstance('inst-1', 'p1', Buffer.from(''))
      
      const [, logHandler] = service.getInstanceWithHandler('inst-1')
      const closeSpy = vi.spyOn(logHandler, 'close')

      await service.destroyInstance('inst-1')

      expect(closeSpy).toHaveBeenCalledOnce()
      expect(() => service.getHandlers('inst-1')).toThrow(/not found/)
    })

    it('should close active session if attached', async () => {
      registry.register(definePlugin({ name: 'p1', setup: () => ({}) }))
      await service.createInstance('inst-1', 'p1', Buffer.from(''))
      
      const session = new PluginSession('inst-1', {}, new Channel(), { ackIntervalSec: 10 })
      const sessionCloseSpy = vi.spyOn(session, 'close').mockResolvedValue()
      
      service.attachSession('inst-1', session)
      
      await service.destroyInstance('inst-1')
      
      expect(sessionCloseSpy).toHaveBeenCalledOnce()
    })

    it('should not throw if instance is not found', async () => {
      await expect(service.destroyInstance('non-existent')).resolves.toBeUndefined()
    })
  })

  describe('session management', () => {
    it('should attach and detach session correctly', async () => {
      registry.register(definePlugin({ name: 'p1', setup: () => ({}) }))
      await service.createInstance('inst-1', 'p1', Buffer.from(''))
      
      const session = new PluginSession('inst-1', {}, new Channel(), { ackIntervalSec: 10 })
      
      service.attachSession('inst-1', session)
      
      // Try attaching another session, should throw
      const session2 = new PluginSession('inst-1', {}, new Channel(), { ackIntervalSec: 10 })
      expect(() => service.attachSession('inst-1', session2)).toThrow(/already has an active stream/)
      
      service.detachSession('inst-1', session)
      
      // Should now be able to attach another session
      expect(() => service.attachSession('inst-1', session2)).not.toThrow()
    })
  })
})
