import type {
  PluginMetadata as SdkPluginMetadata,
  PropertyDef,
  FieldDef,
  PortMetadata as SdkPortMetadata,
  MultiTrackConfig as SdkMultiTrackConfig,
  TrackConfig as SdkTrackConfig,
} from '@/plugin.js'
import {
  DropStrategy,
  EventType,
  ValueType,
  type PluginMetadata as ProtoPluginMetadata,
  type Property as ProtoProperty,
  type Field as ProtoField,
  type PortMetadata as ProtoPortMetadata,
  type MultiTrackConfig as ProtoMultiTrackConfig,
  type TrackConfig as ProtoTrackConfig,
} from '@/proto/plugin.js'

// ---------------------------------------------------------------------------
// Event type mapping
// ---------------------------------------------------------------------------

const EVENT_TYPE_MAP: Record<string, EventType> = {
  signal: EventType.EVENT_TYPE_SIGNAL,
  payload: EventType.EVENT_TYPE_PAYLOAD,
  audio: EventType.EVENT_TYPE_AUDIO,
  video: EventType.EVENT_TYPE_VIDEO,
}

function eventTypeToProto(value: string): EventType {
  return EVENT_TYPE_MAP[value.toLowerCase()] ?? EventType.EVENT_TYPE_UNSPECIFIED
}

// ---------------------------------------------------------------------------
// Value type mapping
// ---------------------------------------------------------------------------

const VALUE_TYPE_MAP: Record<string, ValueType> = {
  string: ValueType.VALUE_TYPE_STRING,
  number: ValueType.VALUE_TYPE_NUMBER,
  float: ValueType.VALUE_TYPE_NUMBER,
  integer: ValueType.VALUE_TYPE_INTEGER,
  int: ValueType.VALUE_TYPE_INTEGER,
  boolean: ValueType.VALUE_TYPE_BOOLEAN,
  bool: ValueType.VALUE_TYPE_BOOLEAN,
  object: ValueType.VALUE_TYPE_OBJECT,
  array: ValueType.VALUE_TYPE_ARRAY,
}

function valueTypeToProto(value: string): ValueType {
  return VALUE_TYPE_MAP[value.toLowerCase()] ?? ValueType.VALUE_TYPE_UNSPECIFIED
}

// ---------------------------------------------------------------------------
// Drop strategy mapping
// ---------------------------------------------------------------------------

const DROP_STRATEGY_MAP: Record<string, DropStrategy> = {
  block_if_full: DropStrategy.DROP_STRATEGY_BLOCK_IF_FULL,
  block: DropStrategy.DROP_STRATEGY_BLOCK_IF_FULL,
  drop_newest: DropStrategy.DROP_STRATEGY_DROP_NEWEST,
  drop_oldest: DropStrategy.DROP_STRATEGY_DROP_OLDEST,
}

function dropStrategyToProto(value: string): DropStrategy {
  return DROP_STRATEGY_MAP[value.toLowerCase()] ?? DropStrategy.DROP_STRATEGY_UNSPECIFIED
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------

export const pluginMetadataToProto = (meta: SdkPluginMetadata): ProtoPluginMetadata => {
  return {
    name: meta.name,
    description: meta.description ?? '',
    schema: JSON.stringify(meta.schema ?? {}),
    inputs: (meta.inputs ?? []).map(propertyToProto),
    outputs: (meta.outputs ?? []).map(propertyToProto),
    ports: (meta.ports ?? []).map(portMetadataToProto),
    multiTrack: meta.multiTrack ? multiTrackConfigToProto(meta.multiTrack) : undefined,
  }
}

function propertyToProto(prop: PropertyDef): ProtoProperty {
  return {
    type: eventTypeToProto(prop.type),
    name: prop.name ?? '',
    fields: (prop.fields ?? []).map(fieldToProto),
  }
}

function fieldToProto(field: FieldDef): ProtoField {
  return {
    key: field.key,
    type: valueTypeToProto(field.type),
    required: field.required ?? false,
  }
}

function portMetadataToProto(port: SdkPortMetadata): ProtoPortMetadata {
  return {
    type: eventTypeToProto(port.type),
    port: port.port,
    name: port.name ?? '',
    description: port.description ?? '',
  }
}

function multiTrackConfigToProto(config: SdkMultiTrackConfig): ProtoMultiTrackConfig {
  return {
    enabled: config.enabled ?? true,
    payload: config.payload ? trackConfigToProto(config.payload) : undefined,
  }
}

function trackConfigToProto(config: SdkTrackConfig): ProtoTrackConfig {
  return {
    enabled: config.enabled ?? true,
    bufferSize: config.bufferSize ?? 128,
    dropStrategy: dropStrategyToProto(config.dropStrategy ?? 'block_if_full'),
    interruptSignals: config.interruptSignals ?? [],
  }
}
