import { describe, it, expect, vi } from 'vitest'
import {
  RemoteLogHandler,
  createPluginLogger,
  mapLogLevel,
} from '@/internal/logger.js'
import { Channel } from '@/internal/channel.js'
import { LogLevel, type RuntimeMessage } from '@/proto/plugin.js'

describe('logger', () => {
  describe('mapLogLevel', () => {
    it('should map valid string levels to LogLevel enum', () => {
      expect(mapLogLevel('debug')).toBe(LogLevel.LOG_LEVEL_DEBUG)
      expect(mapLogLevel('info')).toBe(LogLevel.LOG_LEVEL_INFO)
      expect(mapLogLevel('warn')).toBe(LogLevel.LOG_LEVEL_WARN)
      expect(mapLogLevel('error')).toBe(LogLevel.LOG_LEVEL_ERROR)
    })

    it('should fallback to UNSPECIFIED for unknown levels', () => {
      expect(mapLogLevel('unknown')).toBe(LogLevel.LOG_LEVEL_UNSPECIFIED)
    })
  })

  describe('RemoteLogHandler', () => {
    it('should enqueue and dequeue messages correctly', async () => {
      const handler = new RemoteLogHandler(10)
      const output = new Channel<RuntimeMessage>()
      const ac = new AbortController()
      
      const makeMsg = (data: any) => data as RuntimeMessage
      
      // Start read loop
      const readPromise = handler.readLoop(output, makeMsg, ac.signal)
      
      handler.enqueue(LogLevel.LOG_LEVEL_INFO, 'test message 1')
      handler.enqueue(LogLevel.LOG_LEVEL_ERROR, 'test message 2')
      
      const msg1 = await output.get()
      const msg2 = await output.get()
      
      expect(msg1).toEqual({ level: LogLevel.LOG_LEVEL_INFO, message: 'test message 1' })
      expect(msg2).toEqual({ level: LogLevel.LOG_LEVEL_ERROR, message: 'test message 2' })
      
      ac.abort()
      await readPromise
    })

    it('should drop messages when queue exceeds maxSize', () => {
      const handler = new RemoteLogHandler(2)
      handler.enqueue(LogLevel.LOG_LEVEL_INFO, 'msg 1')
      handler.enqueue(LogLevel.LOG_LEVEL_INFO, 'msg 2')
      handler.enqueue(LogLevel.LOG_LEVEL_INFO, 'msg 3') // should be dropped
      
      // We can access private queue for testing via casting or just dequeue
      const anyHandler = handler as any
      expect(anyHandler.queue).toHaveLength(2)
      expect(anyHandler.queue[0].message).toBe('msg 1')
      expect(anyHandler.queue[1].message).toBe('msg 2')
    })

    it('should stop readLoop when closed', async () => {
      const handler = new RemoteLogHandler()
      const output = new Channel<RuntimeMessage>()
      const ac = new AbortController()
      
      const readPromise = handler.readLoop(output, (msg) => msg as any, ac.signal)
      
      handler.close()
      await readPromise
      // Should resolve cleanly
    })

    it('should resolve blocked dequeue when abort signal fires', async () => {
      const handler = new RemoteLogHandler()
      const output = new Channel<RuntimeMessage>()
      const ac = new AbortController()
      
      const readPromise = handler.readLoop(output, (msg) => msg as any, ac.signal)
      
      // Fire abort to cancel wait
      ac.abort()
      await readPromise
    })
  })

  describe('createPluginLogger', () => {
    it('should forward logs that meet minimum level', () => {
      const handler = new RemoteLogHandler(10)
      const enqueueSpy = vi.spyOn(handler, 'enqueue')
      
      const logger = createPluginLogger(handler, LogLevel.LOG_LEVEL_INFO)
      
      logger.debug('debug msg') // Should be dropped
      logger.info('info msg')
      logger.warn('warn msg')
      logger.error('error msg')
      
      expect(enqueueSpy).toHaveBeenCalledTimes(3)
      expect(enqueueSpy).toHaveBeenCalledWith(LogLevel.LOG_LEVEL_INFO, 'info msg')
      expect(enqueueSpy).toHaveBeenCalledWith(LogLevel.LOG_LEVEL_WARN, 'warn msg')
      expect(enqueueSpy).toHaveBeenCalledWith(LogLevel.LOG_LEVEL_ERROR, 'error msg')
    })
  })
})
