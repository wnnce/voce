// App & Config
export { App, resolveConfig, type Config } from '@/app.js'

// Core Plugin API
export {
  type PluginMetadata,
  type PluginLogger,
  type PluginContext,
  type PluginHandlers,
  type PluginOptions,
  type PluginDefinition,
  definePlugin,
  type Flow,
  pluginRegistry,
  PluginRegistry,
} from '@/plugin.js'

// Testing Utilities
export { MockFlow, PluginTester } from '@/testing.js'

// Schema & Types
export { decodeProperties, encodeProperties } from '@/types.js'
export {
  type Signal,
  createSignal,
  SIGNAL_INTERRUPTER,
  SIGNAL_AGENT_SPEECH_START,
  SIGNAL_AGENT_SPEECH_END,
  SIGNAL_USER_SPEECH_START,
  SIGNAL_USER_SPEECH_END,
  SIGNAL_VAD_USER_SPEECH_START,
  SIGNAL_VAD_USER_SPEECH_END,
} from '@/types.js'
export {
  type Payload,
  createPayload,
  PAYLOAD_ASR_RESULT,
  PAYLOAD_CAPTION,
  PAYLOAD_LLM_CHUNK,
} from '@/types.js'

// Re-export metadata sub-types for plugin authors
export type {
  FieldDef,
  PropertyDef,
  PortMetadata,
  TrackConfig,
  MultiTrackConfig,
} from '@/plugin.js'
