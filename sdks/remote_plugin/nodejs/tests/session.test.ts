import { describe, it, expect, vi, beforeEach } from 'vitest'
import { PluginSession, getAbortSignal, getCurrentCorrelationId } from '@/internal/session.js'
import { Channel } from '@/internal/channel.js'
import { RuntimeMessageType, type RuntimeMessage, LifecycleType } from '@/proto/plugin.js'
import { createPayload, createSignal } from '@/types.js'
import type { PluginHandlers } from '@/plugin.js'

describe('PluginSession', () => {
  let handlers: PluginHandlers
  let outgoing: Channel<RuntimeMessage>
  let session: PluginSession

  beforeEach(() => {
    handlers = {}
    outgoing = new Channel<RuntimeMessage>()
    session = new PluginSession('inst-1', handlers, outgoing, { ackIntervalSec: 1000 })
  })

  describe('Flow interface (sendPayload / sendSignal)', () => {
    it('should push sendPayload messages into the outgoing queue', async () => {
      await session.sendPayload(createPayload('test-pay', { foo: 'bar' }), { port: 2 })
      
      const msg = await outgoing.get()
      expect(msg).toBeDefined()
      expect(msg!.type).toBe(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD)
      expect(msg!.body?.$case).toBe('emitPayload')
      if (msg!.body?.$case === 'emitPayload') {
        expect(msg!.body.emitPayload.port).toBe(2)
        expect(msg!.body.emitPayload.payload?.name).toBe('test-pay')
      }
    })

    it('should push sendSignal messages into the outgoing queue', async () => {
      await session.sendSignal(createSignal('test-sig', { fizz: 'buzz' }))
      
      const msg = await outgoing.get()
      expect(msg).toBeDefined()
      expect(msg!.type).toBe(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL)
      expect(msg!.body?.$case).toBe('emitSignal')
      if (msg!.body?.$case === 'emitSignal') {
        expect(msg!.body.emitSignal.port).toBe(0) // default
        expect(msg!.body.emitSignal.signal?.name).toBe('test-sig')
      }
    })
  })

  describe('processStream and task management', () => {
    it('should properly dispatch a lifecycle message', async () => {
      handlers.onStart = vi.fn().mockResolvedValue(undefined)
      
      const msg: RuntimeMessage = {
        messageId: 'msg-1',
        correlationId: '',
        instanceId: 'inst-1',
        type: RuntimeMessageType.RUNTIME_MESSAGE_TYPE_LIFECYCLE,
        metadata: {},
        body: {
          $case: 'lifecycle',
          lifecycle: { type: LifecycleType.LIFECYCLE_TYPE_START }
        }
      }
      
      const stream = (async function* () {
        yield msg
      })()
      
      await session.processStream(stream)
      
      expect(handlers.onStart).toHaveBeenCalledOnce()
      
      // Should have sent an ACK and an OK report
      const ack = await outgoing.get()
      expect(ack!.type).toBe(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_ACK)
      expect(ack!.correlationId).toBe('msg-1')
      
      const report = await outgoing.get()
      expect(report!.type).toBe(RuntimeMessageType.RUNTIME_MESSAGE_TYPE_REPORT)
      expect(report!.body?.$case).toBe('report')
    })

    it('should inject getAbortSignal() and getCurrentCorrelationId() into the context', async () => {
      let capturedSignal: AbortSignal | undefined
      let capturedId: string | undefined
      let wasAbortedDuringHandler: boolean | undefined
      
      handlers.onPayload = vi.fn().mockImplementation(async () => {
        capturedSignal = getAbortSignal()
        capturedId = getCurrentCorrelationId()
        wasAbortedDuringHandler = capturedSignal?.aborted
      })
      
      const msg: RuntimeMessage = {
        messageId: 'msg-2',
        correlationId: '',
        instanceId: 'inst-1',
        type: RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD,
        metadata: {},
        body: {
          $case: 'payload',
          payload: { name: 'test-pay', properties: Buffer.from('') }
        }
      }
      
      const stream = (async function* () {
        yield msg
      })()
      
      await session.processStream(stream)
      
      expect(handlers.onPayload).toHaveBeenCalledOnce()
      expect(capturedSignal).toBeDefined()
      expect(capturedSignal).toBeInstanceOf(AbortSignal)
      expect(wasAbortedDuringHandler).toBe(false)
      expect(capturedId).toBe('msg-2')
    })

    it('should abort the task if a CANCEL message is received', async () => {
      let capturedSignal: AbortSignal | undefined
      
      handlers.onSignal = vi.fn().mockImplementation(async () => {
        capturedSignal = getAbortSignal()
        // Simulate long running task
        await new Promise(resolve => setTimeout(resolve, 50))
      })
      
      const msg1: RuntimeMessage = {
        messageId: 'msg-3',
        correlationId: '',
        instanceId: 'inst-1',
        type: RuntimeMessageType.RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL,
        metadata: {},
        body: {
          $case: 'signal',
          signal: { name: 'test-sig', properties: Buffer.from('') }
        }
      }
      
      const msgCancel: RuntimeMessage = {
        messageId: 'msg-4',
        correlationId: 'msg-3',
        instanceId: 'inst-1',
        type: RuntimeMessageType.RUNTIME_MESSAGE_TYPE_CANCEL,
        metadata: {},
        body: undefined,
      }
      
      const stream = (async function* () {
        yield msg1
        yield msgCancel // yield cancel immediately
      })()
      
      await session.processStream(stream)
      
      expect(handlers.onSignal).toHaveBeenCalledOnce()
      expect(capturedSignal?.aborted).toBe(true)
    })
  })
})
