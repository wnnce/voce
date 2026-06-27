/**
 * Bounded async queue with sentinel-based termination.
 *
 * Used internally for gRPC stream outgoing messages and log buffering.
 * A `null` value signals the queue is closed; subsequent `get()` calls
 * will return `null` immediately once all buffered items are drained.
 */
export class AsyncQueue<T> {
  private readonly buffer: (T | null)[] = []
  private readonly maxSize: number
  private closed = false

  /** Pending `get()` callers waiting for an item. */
  private waiters: Array<(value: T | null) => void> = []

  constructor(maxSize = 0) {
    this.maxSize = maxSize
  }

  /**
   * Enqueue an item. If the queue is full, the item is silently dropped.
   * Returns `true` if the item was enqueued, `false` if dropped or closed.
   */
  put(item: T | null): boolean {
    if (this.closed && item !== null) return false

    // If someone is waiting, deliver directly.
    if (this.waiters.length > 0) {
      const resolve = this.waiters.shift()!
      resolve(item)
      if (item === null) this.closed = true
      return true
    }

    // Bounded: drop if full (non-blocking).
    if (this.maxSize > 0 && this.buffer.length >= this.maxSize && item !== null) {
      return false
    }

    this.buffer.push(item)
    if (item === null) this.closed = true
    return true
  }

  /**
   * Dequeue an item. Resolves when an item is available.
   * Returns `null` when the queue has been closed and fully drained.
   */
  get(): Promise<T | null> {
    if (this.buffer.length > 0) {
      return Promise.resolve(this.buffer.shift()!)
    }
    if (this.closed) {
      return Promise.resolve(null)
    }
    return new Promise<T | null>((resolve) => {
      this.waiters.push(resolve)
    })
  }

  /** Close the queue. All pending and future `get()` calls will resolve with `null`. */
  close(): void {
    if (this.closed) return
    this.closed = true
    for (const resolve of this.waiters) {
      resolve(null)
    }
    this.waiters = []
  }

  get size(): number {
    return this.buffer.length
  }

  get isClosed(): boolean {
    return this.closed
  }
}
