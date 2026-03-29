package protocol

import (
	"encoding/binary"
	"errors"

	"github.com/wnnce/voce/pkg/pool"
)

const PacketHeaderSize = 8

const (
	MagicNumber1 = 0x56 // 'V'
	MagicNumber2 = 0x43 // 'C'
)

type (
	PacketType   byte
	PacketEncode byte
)

const (
	EncodeRaw  PacketEncode = 0x00
	EncodeJSON PacketEncode = 0x01
)

const (
	TypeAudio            PacketType = 0x01
	TypeError            PacketType = 0x02
	TypeText             PacketType = 0x03
	TypeClose            PacketType = 0x04
	TypeInterrupter      PacketType = 0x11
	TypeCaption          PacketType = 0x12
	TypeUserSpeechStart  PacketType = 0x13
	TypeUserSpeechEnd    PacketType = 0x14
	TypeAgentSpeechStart PacketType = 0x15
	TypeAgentSpeechEnd   PacketType = 0x16
)

var (
	ErrInvalidHeader   = errors.New("invalid packet header")
	ErrMagicMismatch   = errors.New("magic number mismatch")
	ErrPayloadMismatch = errors.New("payload size mismatch")
)

var (
	packetPool = pool.NewTypedPool[*Packet](func() *Packet {
		return &Packet{}
	})
)

func AcquirePacket() *Packet {
	return packetPool.Acquire()
}

func ReleasePacket(p *Packet) {
	packetPool.Release(p)
}

type Packet struct {
	Type    PacketType
	Encode  PacketEncode
	Size    uint32
	Payload []byte
	header  [PacketHeaderSize]byte
}

func (p *Packet) Recycle() {
	p.Type = 0x00
	p.Encode = EncodeRaw
	p.Size = 0
	if cap(p.Payload) <= 64*1024 {
		p.Payload = p.Payload[:0]
	} else {
		p.Payload = nil
	}
	for i := 0; i < PacketHeaderSize; i++ {
		p.header[i] = 0
	}
}

func (p *Packet) SetPayload(data []byte) {
	required := len(data)
	if required == 0 {
		p.Payload = p.Payload[:0]
		p.Size = 0
		return
	}
	if cap(p.Payload) < required {
		p.Payload = make([]byte, required)
	}
	p.Payload = p.Payload[:required]
	copy(p.Payload, data)
	p.Size = uint32(required)
}

func (p *Packet) Header() []byte {
	p.header[0] = MagicNumber1
	p.header[1] = MagicNumber2
	p.header[2] = byte(p.Type)
	p.header[3] = byte(p.Encode)
	binary.BigEndian.PutUint32(p.header[4:8], p.Size)
	return p.header[:]
}

func (p *Packet) Marshal() []byte {
	header := p.Header()
	size := len(p.Payload)
	result := make([]byte, PacketHeaderSize+size)
	copy(result, header)
	copy(result[PacketHeaderSize:], p.Payload)
	return result
}

func (p *Packet) Unmarshal(content []byte) error {
	if len(content) < PacketHeaderSize {
		return ErrInvalidHeader
	}
	if content[0] != MagicNumber1 || content[1] != MagicNumber2 {
		return ErrMagicMismatch
	}
	p.Type = PacketType(content[2])
	p.Encode = PacketEncode(content[3])
	p.Size = binary.BigEndian.Uint32(content[4:8])
	size := len(content[PacketHeaderSize:])
	if int(p.Size) != size {
		return ErrPayloadMismatch
	}
	p.SetPayload(content[PacketHeaderSize:])
	return nil
}
