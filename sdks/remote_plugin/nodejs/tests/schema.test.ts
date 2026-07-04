import { describe, it, expect } from 'vitest'
import {
  createSignal,
  createPayload,
  encodeProperties,
  decodeProperties,
} from '@/types.js'

describe('Properties functions', () => {
  it('should serialize to JSON bytes and back', () => {
    const original = { hello: 'world', num: 42 }
    const bytes = encodeProperties(original)
    const decoded = decodeProperties(bytes)
    expect(decoded).toEqual(original)
  })

  it('should decode empty bytes to empty object', () => {
    expect(decodeProperties(Buffer.alloc(0))).toEqual({})
  })
})

describe('Signal', () => {
  it('should create from factory', () => {
    const signal = createSignal('user_speech_start', { ts: 1234 })
    expect(signal.name).toBe('user_speech_start')
    expect(signal.properties['ts']).toBe(1234)
  })

  it('should allow empty properties', () => {
    const signal = createSignal('empty')
    expect(signal.properties).toEqual({})
  })
})

describe('Payload', () => {
  it('should create from factory', () => {
    const payload = createPayload('llm_chunk', { chunk: 'hi', index: 0 })
    expect(payload.name).toBe('llm_chunk')
    expect(payload.properties['chunk']).toBe('hi')
    expect(payload.properties['index']).toBe(0)
  })

  it('should encode payload properties', () => {
    const payload = createPayload('test', { key: 'val' })
    const bytes = encodeProperties(payload.properties)
    expect(JSON.parse(bytes.toString())).toEqual({ key: 'val' })
  })
})
