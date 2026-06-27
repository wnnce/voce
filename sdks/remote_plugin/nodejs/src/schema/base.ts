import type { z } from 'zod'

/**
 * Base class for named property bags (Signal, Payload).
 *
 * Provides typed property access with optional zod validation.
 */
export class Properties {
  readonly name: string
  readonly properties: Record<string, unknown>

  constructor(name: string, properties: Record<string, unknown> = {}) {
    this.name = name
    this.properties = properties
  }

  /** Get a raw property value. */
  get<T = unknown>(key: string, defaultValue?: T): T | undefined {
    const val = this.properties[key]
    return val !== undefined ? (val as T) : defaultValue
  }

  /** Get a property value validated through a zod schema. */
  getAs<S extends z.ZodType>(key: string, schema: S): z.infer<S> | undefined {
    const val = this.properties[key]
    if (val === undefined) return undefined
    return schema.parse(val)
  }

  /** Validate the entire properties bag against a zod schema. */
  bind<S extends z.ZodType>(schema: S): z.infer<S> {
    return schema.parse(this.properties)
  }

  /** Return a plain copy of the properties dict. */
  toDict(): Record<string, unknown> {
    return { ...this.properties }
  }

  /** Serialize properties to JSON bytes (UTF-8 Buffer). */
  toJsonBytes(): Buffer {
    return Buffer.from(JSON.stringify(this.properties), 'utf-8')
  }
}

/** Decode a JSON-encoded Buffer into a properties dict. */
export function decodeJsonBytes(data: Buffer | Uint8Array): Record<string, unknown> {
  if (!data || data.length === 0) return {}
  const parsed: unknown = JSON.parse(Buffer.from(data).toString('utf-8'))
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    throw new Error('properties must decode to a JSON object')
  }
  return parsed as Record<string, unknown>
}

/** Encode a properties dict to JSON bytes (UTF-8 Buffer). */
export function encodeJsonBytes(value: Record<string, unknown>): Buffer {
  return Buffer.from(JSON.stringify(value), 'utf-8')
}
