export type PropertyType = 'string' | 'number' | 'integer' | 'boolean' | 'array' | 'object' | 'null' | '';

export const EventType = {
  SIGNAL: 0x1,
  PAYLOAD: 0x2,
  AUDIO: 0x3,
  VIDEO: 0x4,
} as const;

export type EventType = (typeof EventType)[keyof typeof EventType];

export interface Field {
  key: string;
  type: PropertyType;
  required: boolean;
}

export interface Property {
  prefix: string;
  name: string;
  fields?: Field[];
}

export interface PortMetadata {
  type: EventType;
  port: number;
  name: string;
  description?: string;
}

export interface PluginInfo {
  name: string;
  description?: string;
  schema?: Record<string, unknown>; // JSON Schema
  inputs?: Property[];
  outputs?: Property[];
  ports?: PortMetadata[];
}

export interface NodeConfig {
  id: string;
  name: string;
  plugin: string;
  config: Record<string, unknown>;
  metadata: Record<string, unknown>;
}

export interface EdgeConfig {
  source: string;
  source_port: number;
  target: string;
  type: EventType;
}

export interface WorkflowConfig {
  id: string;
  name: string;
  version: string;
  head: string;
  nodes: NodeConfig[];
  edges: EdgeConfig[];
}

export interface Result<T> {
  code: number;
  message: string;
  data: T;
}
