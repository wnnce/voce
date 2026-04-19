package machine

import (
	"errors"
	"log/slog"
	"sync/atomic"

	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/protocol"
)

type MessageHandler func(key protocol.SessionKey, packet *protocol.Packet)

type Connection struct {
	gws.BuiltinEventHandler
	socket atomic.Pointer[gws.Conn]
	state  atomic.Int32
	handle MessageHandler
}

func machinePacketTypeName(t protocol.PacketType) string {
	switch t {
	case protocol.TypeAudio:
		return "audio"
	case protocol.TypePause:
		return "pause"
	case protocol.TypeResume:
		return "resume"
	case protocol.TypeClose:
		return "close"
	default:
		return "unknown"
	}
}

func NewConnection(handle MessageHandler) *Connection {
	return &Connection{
		handle: handle,
	}
}

func (c *Connection) Write(key protocol.SessionKey, packet *protocol.Packet) error {
	socket := c.socket.Load()
	if c.State() != protocol.ConnectionActive || socket == nil {
		return errors.New("connection is not active")
	}
	return socket.Writev(gws.OpcodeBinary, key[:], packet.Header(), packet.Payload)
}

func (c *Connection) State() protocol.ConnectionState {
	return protocol.ConnectionState(c.state.Load())
}

func (c *Connection) OnOpen(socket *gws.Conn) {
	slog.Info("machine pool connection established")
	c.socket.Store(socket)
	c.state.Store(int32(protocol.ConnectionActive))
}

func (c *Connection) OnClose(_ *gws.Conn, err error) {
	c.state.Store(int32(protocol.ConnectionClosed))
	c.socket.Store(nil)
}

func (c *Connection) OnMessage(_ *gws.Conn, message *gws.Message) {
	body := message.Bytes()
	defer message.Close()
	if c.handle == nil || message.Opcode != gws.OpcodeBinary || len(body) < protocol.SessionKeySize {
		slog.Warn("machine dropped invalid pool message", "opcode", message.Opcode, "size", len(body), "hasHandler", c.handle != nil)
		return
	}
	key := protocol.SessionKey(body[:protocol.SessionKeySize])
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	if err := packet.Unmarshal(body[protocol.SessionKeySize:]); err != nil {
		slog.Warn("packet unmarshal failed", "error", err)
		return
	}
	c.handle(key, packet)
}
