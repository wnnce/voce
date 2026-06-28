import { describe, it, expect } from 'vitest'
import { pluginMetadataToProto } from '@/internal/converters.js'
import { EventType, ValueType, DropStrategy } from '@/proto/plugin.js'
import type { PluginMetadata } from '@/plugin.js'

describe('converters', () => {
  describe('pluginMetadataToProto', () => {
    it('should convert basic metadata correctly', () => {
      const meta: PluginMetadata = {
        name: 'test-plugin',
        description: 'A test plugin',
        schema: { type: 'object', properties: { foo: { type: 'string' } } },
      }

      const proto = pluginMetadataToProto(meta)

      expect(proto.name).toBe('test-plugin')
      expect(proto.description).toBe('A test plugin')
      expect(JSON.parse(proto.schema)).toEqual({
        type: 'object',
        properties: { foo: { type: 'string' } },
      })
      expect(proto.inputs).toEqual([])
      expect(proto.outputs).toEqual([])
      expect(proto.ports).toEqual([])
      expect(proto.multiTrack).toBeUndefined()
    })

    it('should convert inputs and outputs with properties and fields', () => {
      const meta: PluginMetadata = {
        name: 'io-plugin',
        inputs: [
          {
            type: 'signal',
            name: 'test-signal',
            fields: [
              { key: 'param1', type: 'string', required: true },
              { key: 'param2', type: 'integer' },
            ],
          },
        ],
        outputs: [
          {
            type: 'payload',
            name: 'test-payload',
          },
        ],
      }

      const proto = pluginMetadataToProto(meta)

      expect(proto.inputs).toHaveLength(1)
      expect(proto.inputs[0].type).toBe(EventType.EVENT_TYPE_SIGNAL)
      expect(proto.inputs[0].name).toBe('test-signal')
      expect(proto.inputs[0].fields).toHaveLength(2)
      expect(proto.inputs[0].fields[0]).toEqual({
        key: 'param1',
        type: ValueType.VALUE_TYPE_STRING,
        required: true,
      })
      expect(proto.inputs[0].fields[1]).toEqual({
        key: 'param2',
        type: ValueType.VALUE_TYPE_INTEGER,
        required: false,
      })

      expect(proto.outputs).toHaveLength(1)
      expect(proto.outputs[0].type).toBe(EventType.EVENT_TYPE_PAYLOAD)
      expect(proto.outputs[0].name).toBe('test-payload')
      expect(proto.outputs[0].fields).toEqual([])
    })

    it('should correctly convert ports', () => {
      const meta: PluginMetadata = {
        name: 'port-plugin',
        ports: [
          {
            type: 'audio',
            port: 1,
            name: 'audio-out',
            description: 'Main audio output',
          },
        ],
      }

      const proto = pluginMetadataToProto(meta)

      expect(proto.ports).toHaveLength(1)
      expect(proto.ports[0]).toEqual({
        type: EventType.EVENT_TYPE_AUDIO,
        port: 1,
        name: 'audio-out',
        description: 'Main audio output',
      })
    })

    it('should correctly convert multiTrack config', () => {
      const meta: PluginMetadata = {
        name: 'multitrack-plugin',
        multiTrack: {
          enabled: false,
          payload: {
            enabled: true,
            bufferSize: 256,
            dropStrategy: 'drop_oldest',
            interruptSignals: ['interrupt'],
          },
        },
      }

      const proto = pluginMetadataToProto(meta)

      expect(proto.multiTrack).toBeDefined()
      expect(proto.multiTrack?.enabled).toBe(false)
      expect(proto.multiTrack?.payload).toEqual({
        enabled: true,
        bufferSize: 256,
        dropStrategy: DropStrategy.DROP_STRATEGY_DROP_OLDEST,
        interruptSignals: ['interrupt'],
      })
    })

    it('should handle undefined optional fields with defaults', () => {
      const meta: PluginMetadata = {
        name: 'minimal',
      }

      const proto = pluginMetadataToProto(meta)
      expect(proto.description).toBe('')
      expect(proto.schema).toBe('{}')
      expect(proto.inputs).toEqual([])
      expect(proto.outputs).toEqual([])
      expect(proto.ports).toEqual([])
    })

    it('should map unknown types to UNSPECIFIED', () => {
      const meta: PluginMetadata = {
        name: 'unknown-types',
        inputs: [
          {
            type: 'unknown_event' as any,
            fields: [{ key: 'f1', type: 'unknown_value' as any }],
          },
        ],
        multiTrack: {
          payload: {
            dropStrategy: 'unknown_strategy' as any,
          },
        },
      }

      const proto = pluginMetadataToProto(meta)
      expect(proto.inputs[0].type).toBe(EventType.EVENT_TYPE_UNSPECIFIED)
      expect(proto.inputs[0].fields[0].type).toBe(ValueType.VALUE_TYPE_UNSPECIFIED)
      expect(proto.multiTrack?.payload?.dropStrategy).toBe(
        DropStrategy.DROP_STRATEGY_UNSPECIFIED,
      )
    })
  })
})
