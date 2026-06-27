import { Properties, decodeJsonBytes } from './base.js'

// Well-known signal names
export const SIGNAL_INTERRUPTER = 'interrupter'
export const SIGNAL_AGENT_SPEECH_START = 'agent_speech_start'
export const SIGNAL_AGENT_SPEECH_END = 'agent_speech_end'
export const SIGNAL_USER_SPEECH_START = 'user_speech_start'
export const SIGNAL_USER_SPEECH_END = 'user_speech_end'
export const SIGNAL_VAD_USER_SPEECH_START = 'vad_user_speech_start'
export const SIGNAL_VAD_USER_SPEECH_END = 'vad_user_speech_end'

export class Signal extends Properties {
  /** Create a Signal from a name and JSON-encoded bytes. */
  static fromJsonBytes(name: string, data: Buffer | Uint8Array): Signal {
    return new Signal(name, decodeJsonBytes(data))
  }

  /** Create a Signal from a name and a plain object. */
  static fromObject(name: string, obj: Record<string, unknown>): Signal {
    return new Signal(name, obj)
  }
}
