// ---------------------------------------------------------------------------
// Base Properties
// ---------------------------------------------------------------------------

/**
 * Decode a JSON-encoded Buffer into a generic object.
 */
export const decodeProperties = (data: Buffer | Uint8Array): Record<string, unknown> => {
  if (!data || data.length === 0) return {}
  const parsed: unknown = JSON.parse(Buffer.from(data).toString('utf-8'))
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    throw new Error('properties must decode to a JSON object')
  }
  return parsed as Record<string, unknown>
}

/**
 * Encode a generic object to JSON bytes (UTF-8 Buffer).
 */
export const encodeProperties = (value: Record<string, unknown>): Buffer => {
  return Buffer.from(JSON.stringify(value), 'utf-8')
}

// ---------------------------------------------------------------------------
// Signal
// ---------------------------------------------------------------------------

export const SIGNAL_INTERRUPTER = 'interrupter'
export const SIGNAL_AGENT_SPEECH_START = 'agent_speech_start'
export const SIGNAL_AGENT_SPEECH_END = 'agent_speech_end'
export const SIGNAL_USER_SPEECH_START = 'user_speech_start'
export const SIGNAL_USER_SPEECH_END = 'user_speech_end'
export const SIGNAL_VAD_USER_SPEECH_START = 'vad_user_speech_start'
export const SIGNAL_VAD_USER_SPEECH_END = 'vad_user_speech_end'

/**
 * Signal represents a discrete event in the DAG.
 */
export interface Signal {
  name: string
  properties: Record<string, unknown>
}

/** Helper to create a new Signal object. */
export const createSignal = (name: string, properties: Record<string, unknown> = {}): Signal => {
  return { name, properties }
}

// ---------------------------------------------------------------------------
// Payload
// ---------------------------------------------------------------------------

export const PAYLOAD_ASR_RESULT = 'asr_result'
export const PAYLOAD_CAPTION = 'caption'
export const PAYLOAD_LLM_CHUNK = 'llm_chunk'

/**
 * Payload represents a data chunk traversing the DAG.
 */
export interface Payload {
  name: string
  properties: Record<string, unknown>
}

/** Helper to create a new Payload object. */
export const createPayload = (name: string, properties: Record<string, unknown> = {}): Payload => {
  return { name, properties }
}
