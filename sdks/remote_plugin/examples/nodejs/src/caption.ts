import { z } from 'zod'
import { definePlugin, createPayload, SIGNAL_INTERRUPTER } from '@voce/plugin-sdk'

const ROLE_USER = 'user'
const ROLE_ASSISTANT = 'assistant'

/**
 * Caption plugin:
 * Buffers `llm_chunk` payloads into full sentences and formats `asr_result` texts.
 * Outputs a `caption` payload.
 */
export const captionPlugin = definePlugin({
  name: 'caption',
  description: 'Caption formatting plugin',
  // No specific configuration required for this plugin
  configSchema: z.object({}),

  inputs: [
    {
      type: 'payload',
      name: 'asr_result',
      fields: [
        { key: 'text', type: 'string', required: true },
        { key: 'is_final', type: 'boolean', required: true },
      ],
    },
    {
      type: 'payload',
      name: 'llm_chunk',
      fields: [
        { key: 'sentence', type: 'string', required: true },
        { key: 'is_final', type: 'boolean', required: true },
      ],
    },
  ],

  outputs: [
    {
      type: 'payload',
      name: 'caption',
      fields: [
        { key: 'caption', type: 'object', required: true },
      ],
    },
  ],

  setup(context) {
    // Closure state bound to the lifecycle of this specific plugin instance (session).
    let builder = ''

    return {
      async onStart() {
        context.logger.info('Caption extension onStart')
      },

      async onStop() {
        context.logger.info('Caption extension onStop')
      },

      async onSignal(signal, flow) {
        if (signal.name === SIGNAL_INTERRUPTER) {
          builder = ''
        }
        await flow.sendSignal(signal)
      },

      async onPayload(payload, flow) {
        let text = ''
        let role = ''
        let isFinal = false

        if (payload.name === 'asr_result') {
          text = (payload.properties.text as string) || ''
          isFinal = !!payload.properties.is_final
          role = ROLE_USER
        } else if (payload.name === 'llm_chunk') {
          const sentence = (payload.properties.sentence as string) || ''
          isFinal = !!payload.properties.is_final
          role = ROLE_ASSISTANT

          builder += sentence
          text = builder

          if (isFinal) {
            builder = ''
          }
        } else {
          // Ignore other payloads
          return
        }

        const outputData = createPayload('caption', {
          caption: {
            text,
            role,
            is_final: isFinal,
          },
        })

        try {
          await flow.sendPayload(outputData)
        } catch (err) {
          context.logger.error(`Output payload send failed: ${err}`)
        }
      },
    }
  },
})
