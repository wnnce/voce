import { describe, it, expect } from 'vitest'
import { Signal, Payload, Properties } from '../src/schema/index.js'

describe('Properties', () => {
  it('should store name and properties', () => {
    const props = new Properties('test', { key: 'value', num: 42 })
    expect(props.name).toBe('test')
    expect(props.get('key')).toBe('value')
    expect(props.get('num')).toBe(42)
    expect(props.get('missing')).toBeUndefined()
    expect(props.get('missing', 'default')).toBe('default')
  })

  it('should serialize to JSON bytes and back', () => {
    const props = new Properties('test', { hello: 'world' })
    const bytes = props.toJsonBytes()
    expect(JSON.parse(bytes.toString('utf-8'))).toEqual({ hello: 'world' })
  })

  it('should return a plain dict copy', () => {
    const original = { a: 1, b: 'two' }
    const props = new Properties('x', original)
    const dict = props.toDict()
    expect(dict).toEqual(original)
    // Mutating the copy should not affect the original
    dict.c = 3
    expect(props.get('c')).toBeUndefined()
  })
})

describe('Signal', () => {
  it('should create from JSON bytes', () => {
    const data = Buffer.from(JSON.stringify({ confidence: 0.95 }))
    const signal = Signal.fromJsonBytes('interrupter', data)
    expect(signal.name).toBe('interrupter')
    expect(signal.get('confidence')).toBe(0.95)
  })

  it('should create from object', () => {
    const signal = Signal.fromObject('user_speech_start', { ts: 1234 })
    expect(signal.name).toBe('user_speech_start')
    expect(signal.get('ts')).toBe(1234)
  })

  it('should handle empty bytes', () => {
    const signal = Signal.fromJsonBytes('empty', Buffer.alloc(0))
    expect(signal.properties).toEqual({})
  })
})

describe('Payload', () => {
  it('should create from JSON bytes', () => {
    const data = Buffer.from(JSON.stringify({ text: 'hello' }))
    const payload = Payload.fromJsonBytes('asr_result', data)
    expect(payload.name).toBe('asr_result')
    expect(payload.get('text')).toBe('hello')
  })

  it('should create from object', () => {
    const payload = Payload.fromObject('llm_chunk', { chunk: 'hi', index: 0 })
    expect(payload.name).toBe('llm_chunk')
    expect(payload.get('chunk')).toBe('hi')
    expect(payload.get('index')).toBe(0)
  })

  it('should serialize properties to JSON bytes', () => {
    const payload = new Payload('test', { key: 'val' })
    const bytes = payload.toJsonBytes()
    expect(JSON.parse(bytes.toString())).toEqual({ key: 'val' })
  })
})
