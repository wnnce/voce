use std::convert::TryFrom;
use anyhow::{Result, Error};
use bytes::{BufMut, BytesMut, Buf};

pub const MAGIC_NUMBER1: u8 = 0x56; // 'V'
pub const MAGIC_NUMBER2: u8 = 0x43; // 'C'
pub const PACKET_HEADER_SIZE: usize = 8;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PacketType {
    Audio = 0x01,
    Error = 0x02,
    Text = 0x03,
    Close = 0x04,
    Interrupter = 0x11,
    Caption = 0x12,
    UserSpeechStart = 0x13,
    UserSpeechEnd = 0x14,
    AgentSpeechStart = 0x15,
    AgentSpeechEnd = 0x16,
    SessionId = 0x17,
}

impl TryFrom<u8> for PacketType {
    type Error = Error;
    fn try_from(v: u8) -> Result<Self> {
        match v {
            0x01 => Ok(PacketType::Audio),
            0x02 => Ok(PacketType::Error),
            0x03 => Ok(PacketType::Text),
            0x04 => Ok(PacketType::Close),
            0x11 => Ok(PacketType::Interrupter),
            0x12 => Ok(PacketType::Caption),
            0x13 => Ok(PacketType::UserSpeechStart),
            0x14 => Ok(PacketType::UserSpeechEnd),
            0x15 => Ok(PacketType::AgentSpeechStart),
            0x16 => Ok(PacketType::AgentSpeechEnd),
            0x17 => Ok(PacketType::SessionId),
            _ => Err(anyhow::anyhow!("Unknown PacketType: {}", v)),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PacketEncode {
    Raw = 0x00,
    Json = 0x01,
}

impl TryFrom<u8> for PacketEncode {
    type Error = Error;
    fn try_from(v: u8) -> Result<Self> {
        match v {
            0x00 => Ok(PacketEncode::Raw),
            0x01 => Ok(PacketEncode::Json),
            _ => Err(anyhow::anyhow!("Unknown PacketEncode: {}", v)),
        }
    }
}

#[derive(Debug, Clone)]
pub struct Packet {
    pub p_type: PacketType,
    pub encode: PacketEncode,
    pub payload: Vec<u8>,
}

impl Packet {
    pub fn new(p_type: PacketType, payload: Vec<u8>) -> Self {
        Self {
            p_type,
            encode: PacketEncode::Raw,
            payload,
        }
    }

    pub fn marshal(&self) -> Vec<u8> {
        let size = self.payload.len() as u32;
        let mut buf = BytesMut::with_capacity(PACKET_HEADER_SIZE + size as usize);
        buf.put_u8(MAGIC_NUMBER1);
        buf.put_u8(MAGIC_NUMBER2);
        buf.put_u8(self.p_type as u8);
        buf.put_u8(self.encode as u8);
        buf.put_u32(size);
        buf.put_slice(&self.payload);
        buf.to_vec()
    }

    pub fn unmarshal(mut data: &[u8]) -> Result<Self> {
        if data.len() < PACKET_HEADER_SIZE {
            return Err(anyhow::anyhow!("Invalid packet length"));
        }
        
        let m1 = data.get_u8();
        let m2 = data.get_u8();
        if m1 != MAGIC_NUMBER1 || m2 != MAGIC_NUMBER2 {
            return Err(anyhow::anyhow!("Magic number mismatch"));
        }

        let p_type = PacketType::try_from(data.get_u8())?;
        let encode = PacketEncode::try_from(data.get_u8())?;
        let size = data.get_u32() as usize;

        if data.len() != size {
            return Err(anyhow::anyhow!("Payload size mismatch: expected {}, got {}", size, data.len()));
        }

        let payload = data.to_vec();
        Ok(Self {
            p_type,
            encode,
            payload,
        })
    }
}
