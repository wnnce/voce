import { Properties, decodeJsonBytes } from './base.js'

// Well-known payload names
export const PAYLOAD_ASR_RESULT = 'asr_result'
export const PAYLOAD_CAPTION = 'caption'
export const PAYLOAD_LLM_CHUNK = 'llm_chunk'

export class Payload extends Properties {
  /** Create a Payload from a name and JSON-encoded bytes. */
  static fromJsonBytes(name: string, data: Buffer | Uint8Array): Payload {
    return new Payload(name, decodeJsonBytes(data))
  }

  /** Create a Payload from a name and a plain object. */
  static fromObject(name: string, obj: Record<string, unknown>): Payload {
    return new Payload(name, obj)
  }
}
