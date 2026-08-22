package machine

import (
	"bytes"
	"testing"

	"github.com/lxzan/gws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
)

func TestConnectionWriteRejectsInactiveConnection(t *testing.T) {
	conn := NewConnection(nil, nil)
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)

	require.Error(t, conn.Write(testSessionKey(1), packet))

	conn.state.Store(int32(protocol.ConnectionActive))
	assert.Error(t, conn.Write(testSessionKey(1), packet))
}

func TestConnectionOnMessage(t *testing.T) {
	key := testSessionKey(1)
	packet := protocol.AcquirePacket()
	packet.Type = protocol.TypeAudio
	packet.Encode = protocol.EncodeRaw
	packet.SetPayload([]byte("audio"))
	body := append(append([]byte{}, key[:]...), packet.Marshal()...)
	protocol.ReleasePacket(packet)

	t.Run("dispatches valid binary packet", func(t *testing.T) {
		var gotKey protocol.SessionKey
		var gotType protocol.PacketType
		var gotPayload []byte
		conn := NewConnection(nil, func(key protocol.SessionKey, packet *protocol.Packet) {
			gotKey = key
			gotType = packet.Type
			gotPayload = append(gotPayload[:0], packet.Payload...)
		})

		message := &gws.Message{Opcode: gws.OpcodeBinary, Data: bytes.NewBuffer(body)}
		conn.OnMessage(nil, message)

		assert.Equal(t, key, gotKey)
		assert.Equal(t, protocol.TypeAudio, gotType)
		assert.Equal(t, []byte("audio"), gotPayload)
		assert.Nil(t, message.Data)
	})

	t.Run("drops invalid messages", func(t *testing.T) {
		tests := []struct {
			name    string
			opcode  gws.Opcode
			body    []byte
			handler MessageHandler
		}{
			{name: "no handler", opcode: gws.OpcodeBinary, body: body},
			{name: "text frame", opcode: gws.OpcodeText, body: body, handler: func(protocol.SessionKey, *protocol.Packet) { t.Fatal("handler called") }},
			{name: "short frame", opcode: gws.OpcodeBinary, body: key[:protocol.SessionKeySize-1], handler: func(protocol.SessionKey, *protocol.Packet) { t.Fatal("handler called") }},
			{name: "invalid packet", opcode: gws.OpcodeBinary, body: append(key[:], []byte("bad")...), handler: func(protocol.SessionKey, *protocol.Packet) { t.Fatal("handler called") }},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				conn := NewConnection(nil, tt.handler)
				message := &gws.Message{Opcode: tt.opcode, Data: bytes.NewBuffer(tt.body)}
				conn.OnMessage(nil, message)
				require.Nil(t, message.Data)
			})
		}
	})
}
