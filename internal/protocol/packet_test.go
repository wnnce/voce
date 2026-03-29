package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPacket_MarshalUnmarshal(t *testing.T) {
	p := AcquirePacket()
	defer ReleasePacket(p)

	p.Type = TypeAgentSpeechStart
	p.Encode = EncodeJSON
	payload := []byte("test-payload-content")
	p.SetPayload(payload)

	// Marshal
	data := p.Marshal()
	assert.Len(t, data, PacketHeaderSize+len(payload))

	// Unmarshal to a new packet
	p2 := AcquirePacket()
	defer ReleasePacket(p2)

	err := p2.Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, p.Type, p2.Type)
	assert.Equal(t, p.Encode, p2.Encode)
	assert.Equal(t, p.Size, p2.Size)
	assert.True(t, bytes.Equal(p.Payload, p2.Payload))
}

func TestPacket_SetPayload(t *testing.T) {
	p := AcquirePacket()
	defer ReleasePacket(p)

	// Small to Large
	p.SetPayload([]byte("short"))
	assert.Len(t, p.Payload, 5)

	oldCap := cap(p.Payload)
	p.SetPayload(make([]byte, 1024))
	assert.Len(t, p.Payload, 1024)
	assert.GreaterOrEqual(t, cap(p.Payload), 1024)

	// Large to Small (should reuse buffer)
	newCap := cap(p.Payload)
	p.SetPayload([]byte("tiny"))
	assert.Len(t, p.Payload, 4)
	assert.Equal(t, newCap, cap(p.Payload), "Should reuse buffer capacity")

	if oldCap < 1024 {
		assert.NotEqual(t, oldCap, newCap, "Should have reallocated for larger payload")
	}
}

func TestPacket_Recycle(t *testing.T) {
	p := AcquirePacket()
	p.Type = TypeCaption
	p.Encode = EncodeJSON
	p.SetPayload([]byte("temporary data"))

	p.Recycle()

	assert.Equal(t, PacketType(0x00), p.Type)
	assert.Equal(t, EncodeRaw, p.Encode)
	assert.Equal(t, uint32(0), p.Size)
	assert.Empty(t, p.Payload)
}

func TestPacket_UnmarshalErrors(t *testing.T) {
	p := AcquirePacket()
	defer ReleasePacket(p)

	t.Run("Invalid Header", func(t *testing.T) {
		err := p.Unmarshal([]byte{0x01, 0x02, 0x03})
		assert.ErrorIs(t, err, ErrInvalidHeader)
	})

	t.Run("Magic Mismatch", func(t *testing.T) {
		data := make([]byte, PacketHeaderSize)
		data[0] = 0x00 // Wrong magic
		err := p.Unmarshal(data)
		assert.ErrorIs(t, err, ErrMagicMismatch)
	})

	t.Run("Payload Mismatch", func(t *testing.T) {
		data := make([]byte, PacketHeaderSize+10)
		data[0] = MagicNumber1
		data[1] = MagicNumber2
		binary.BigEndian.PutUint32(data[4:8], 5) // Expected 5, but got 10
		err := p.Unmarshal(data)
		assert.ErrorIs(t, err, ErrPayloadMismatch)
	})
}

func BenchmarkPacket_Marshal(b *testing.B) {
	p := AcquirePacket()
	defer ReleasePacket(p)
	p.Type = TypeText
	p.SetPayload(make([]byte, 1024))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Marshal()
	}
}

func BenchmarkPacket_Unmarshal(b *testing.B) {
	p := AcquirePacket()
	p.Type = TypeText
	p.SetPayload(make([]byte, 1024))
	data := p.Marshal()
	ReleasePacket(p)

	p2 := AcquirePacket()
	defer ReleasePacket(p2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p2.Unmarshal(data)
	}
}
