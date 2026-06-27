import { describe, it, expect } from 'vitest'
import { AsyncQueue } from '../src/utils/async-queue.js'

describe('AsyncQueue', () => {
  it('should put and get items in order', async () => {
    const q = new AsyncQueue<number>()
    q.put(1)
    q.put(2)
    q.put(3)
    expect(await q.get()).toBe(1)
    expect(await q.get()).toBe(2)
    expect(await q.get()).toBe(3)
  })

  it('should resolve get() when item is put later', async () => {
    const q = new AsyncQueue<string>()
    const promise = q.get()
    q.put('hello')
    expect(await promise).toBe('hello')
  })

  it('should return null when closed', async () => {
    const q = new AsyncQueue<number>()
    q.put(1)
    q.close()
    expect(await q.get()).toBe(1) // buffered item first
    expect(await q.get()).toBeNull() // then null
  })

  it('should resolve pending get() with null on close', async () => {
    const q = new AsyncQueue<number>()
    const promise = q.get()
    q.close()
    expect(await promise).toBeNull()
  })

  it('should drop items when bounded and full', () => {
    const q = new AsyncQueue<number>(2)
    expect(q.put(1)).toBe(true)
    expect(q.put(2)).toBe(true)
    expect(q.put(3)).toBe(false) // dropped
    expect(q.size).toBe(2)
  })

  it('should accept null as sentinel', async () => {
    const q = new AsyncQueue<number>()
    q.put(42)
    q.put(null)
    expect(await q.get()).toBe(42)
    expect(await q.get()).toBeNull()
    expect(q.isClosed).toBe(true)
  })

  it('should reject puts after close', () => {
    const q = new AsyncQueue<number>()
    q.close()
    expect(q.put(1)).toBe(false)
  })
})
