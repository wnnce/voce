import type { Signal } from '../schema/signal.js'
import type { Payload } from '../schema/payload.js'

/**
 * Flow interface used by plugins to emit signals and payloads back into the DAG.
 */
export interface Flow {
  sendPayload(payload: Payload, options?: { port?: number }): Promise<void>
  sendSignal(signal: Signal, options?: { port?: number }): Promise<void>
}
