/** Mirrors internal/protocol/packet.go. Keep byte values in sync. */
export const PacketType = {
  Audio: 0x01,
  Error: 0x02,
  Text: 0x03,
  Close: 0x04,
  Pause: 0x05,
  Resume: 0x06,
  Interrupter: 0x11,
  Caption: 0x12,
  UserSpeechStart: 0x13,
  UserSpeechEnd: 0x14,
  AgentSpeechStart: 0x15,
  AgentSpeechEnd: 0x16,
} as const;

export type PacketTypeValue = (typeof PacketType)[keyof typeof PacketType];

export const PacketEncode = {
  Raw: 0x00,
  Json: 0x01,
} as const;

export type PacketEncodeValue = (typeof PacketEncode)[keyof typeof PacketEncode];

export const PACKET_HEADER_SIZE = 8;
export const MAGIC_1 = 0x56; // 'V'
export const MAGIC_2 = 0x43; // 'C'

export interface Packet {
  type: number;
  encode: number;
  payload: Uint8Array;
}

const typeNames: Record<number, string> = {
  [PacketType.Audio]: 'audio',
  [PacketType.Error]: 'error',
  [PacketType.Text]: 'text',
  [PacketType.Close]: 'close',
  [PacketType.Pause]: 'pause',
  [PacketType.Resume]: 'resume',
  [PacketType.Interrupter]: 'interrupter',
  [PacketType.Caption]: 'caption',
  [PacketType.UserSpeechStart]: 'user_speech_start',
  [PacketType.UserSpeechEnd]: 'user_speech_end',
  [PacketType.AgentSpeechStart]: 'agent_speech_start',
  [PacketType.AgentSpeechEnd]: 'agent_speech_end',
};

export function packetTypeName(type: number): string {
  return typeNames[type] ?? `0x${type.toString(16).padStart(2, '0')}`;
}

/**
 * Serialises a packet into the 8-byte VC header plus payload.
 *
 * Layout: 'V' 'C' | type | encode | size (big-endian uint32) | payload
 */
export function encodePacket(packet: Packet): ArrayBuffer {
  const buffer = new ArrayBuffer(PACKET_HEADER_SIZE + packet.payload.byteLength);
  const view = new DataView(buffer);
  view.setUint8(0, MAGIC_1);
  view.setUint8(1, MAGIC_2);
  view.setUint8(2, packet.type);
  view.setUint8(3, packet.encode);
  view.setUint32(4, packet.payload.byteLength, false);
  new Uint8Array(buffer, PACKET_HEADER_SIZE).set(packet.payload);
  return buffer;
}

export interface DecodeResult {
  packet: Packet | null;
  error?: 'too-short' | 'magic' | 'size';
}

/**
 * Parses a websocket frame. Header validation mirrors UnmarshalHeader on the
 * server: magic bytes first, then the declared size must equal the remaining
 * length. A bad frame is reported, never thrown — a corrupt frame must not
 * tear down the connection.
 */
export function decodePacket(data: ArrayBuffer): DecodeResult {
  if (data.byteLength < PACKET_HEADER_SIZE) {
    return { packet: null, error: 'too-short' };
  }
  const view = new DataView(data);
  if (view.getUint8(0) !== MAGIC_1 || view.getUint8(1) !== MAGIC_2) {
    return { packet: null, error: 'magic' };
  }
  const size = view.getUint32(4, false);
  if (size !== data.byteLength - PACKET_HEADER_SIZE) {
    return { packet: null, error: 'size' };
  }
  return {
    packet: {
      type: view.getUint8(2),
      encode: view.getUint8(3),
      payload: new Uint8Array(data, PACKET_HEADER_SIZE, size),
    },
  };
}

export interface CaptionPayload {
  text: string;
  role: 'user' | 'assistant';
  is_final: boolean;
}

/** Decodes a JSON-encoded payload, returning null on malformed input. */
export function decodeJSON<T>(packet: Packet): T | null {
  if (packet.encode !== PacketEncode.Json) return null;
  try {
    const text = new TextDecoder().decode(packet.payload);
    return JSON.parse(text) as T;
  } catch {
    return null;
  }
}

/** Encodes a UTF-8 JSON payload. */
export function jsonPayload(value: unknown): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(value));
}
