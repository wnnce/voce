// App & Config
export { App, resolveConfig, type Config } from './app.js'

// Core
export { AsyncPlugin, type PluginMetadata, type PluginLogger } from './core/plugin.js'
export type { Flow } from './core/flow.js'
export {
  plugin,
  registerPlugin,
  pluginRegistry,
  PluginRegistry,
  type PluginOptions,
} from './core/registry.js'
export { MockFlow, PluginTester } from './core/tester.js'

// Schema
export { Signal, Payload, Properties } from './schema/index.js'
export {
  SIGNAL_INTERRUPTER,
  SIGNAL_AGENT_SPEECH_START,
  SIGNAL_AGENT_SPEECH_END,
  SIGNAL_USER_SPEECH_START,
  SIGNAL_USER_SPEECH_END,
  SIGNAL_VAD_USER_SPEECH_START,
  SIGNAL_VAD_USER_SPEECH_END,
  PAYLOAD_ASR_RESULT,
  PAYLOAD_CAPTION,
  PAYLOAD_LLM_CHUNK,
} from './schema/index.js'

// Re-export metadata sub-types for plugin authors
export type {
  FieldDef,
  PropertyDef,
  PortMetadata,
  TrackConfig,
  MultiTrackConfig,
} from './core/plugin.js'
