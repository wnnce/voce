package machine

import (
	"errors"
	"log/slog"
	"sync/atomic"

	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/protocol"
)

type MessageHandler func(key protocol.SessionKey, packet *protocol.Packet)

// Connection represents an inbound pool connection from the gateway to the machine.
type Connection struct {
	gws.BuiltinEventHandler
	manager *ConnectionManager
	socket  atomic.Pointer[gws.Conn]
	state   atomic.Int32
	handle  MessageHandler
}

// NewConnection creates a new pool connection instance.
func NewConnection(manager *ConnectionManager, handle MessageHandler) *Connection {
	return &Connection{
		manager: manager,
		handle:  handle,
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
	if c.manager != nil {
		c.manager.Store(c)
	}
}

func (c *Connection) OnClose(_ *gws.Conn, err error) {
	c.state.Store(int32(protocol.ConnectionClosed))
	c.socket.Store(nil)
	if c.manager != nil {
		c.manager.Remove(c)
	}
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
