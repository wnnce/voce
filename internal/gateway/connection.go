package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/buf"
	"github.com/wnnce/voce/pkg/pool"
)

type MessageDispatcher func(key protocol.SessionKey, data []byte)

var (
	ErrConnectionNotActive = errors.New("connection is not active")
	ErrNilNBHTTPEngine     = errors.New("nbhttp engine is nil")
	ErrInvalidPoolAddress  = errors.New("invalid machine pool address")
	ErrDialMachinePool     = errors.New("dial machine pool failed")
)

var (
	bufferPool = pool.NewTypedPool[*buf.Buffer](func() *buf.Buffer {
		return &buf.Buffer{
			Buf:        make([]byte, 4*1024),
			RecycleCap: 64 * 1024,
		}
	})
)

// Connection represents a persistent WebSocket connection to a machine's data pool.
type Connection struct {
	machineID  string
	ctx        context.Context
	addr       *url.URL
	dialer     *websocket.Dialer
	state      atomic.Int32
	socket     atomic.Pointer[websocket.Conn]
	dispatcher MessageDispatcher
	slot       int
}

// NewConnection creates and initializes a new pool connection.
func NewConnection(
	ctx context.Context,
	engine *nbhttp.Engine,
	machineID, address string,
	slot int,
	dispatcher MessageDispatcher,
) (*Connection, error) {
	u, err := url.Parse("ws://" + address + "/pool")
	if err != nil {
		return nil, ErrInvalidPoolAddress
	}
	conn := &Connection{
		machineID:  machineID,
		ctx:        ctx,
		addr:       u,
		slot:       slot,
		dispatcher: dispatcher,
	}
	if engine == nil {
		return nil, ErrNilNBHTTPEngine
	}
	upgrade := websocket.NewUpgrader()
	upgrade.OnMessage(conn.OnMessage)
	upgrade.OnClose(conn.OnClose)
	upgrade.OnOpen(conn.OnOpen)
	dialer := &websocket.Dialer{
		Engine:      engine,
		Upgrader:    upgrade,
		DialTimeout: 1 * time.Second,
	}
	conn.dialer = dialer
	conn.state.Store(int32(protocol.ConnectionConnecting))
	if err = conn.Connect(); err != nil {
		return nil, err
	}
	return conn, nil
}

// Connect initiates the WebSocket handshake.
func (c *Connection) Connect() error {
	q := c.addr.Query()
	q.Set("slot", strconv.Itoa(c.slot))
	c.addr.RawQuery = q.Encode()
	slog.Info("gateway dialing machine pool", "machineID", c.machineID, "url", c.addr.String())
	//nolint:bodyclose // nbio
	_, _, err := c.dialer.DialContext(c.ctx, c.addr.String(), nil)
	if err != nil {
		slog.Error("gateway dial machine pool failed", "machineID", c.machineID, "url", c.addr.String(), "error", err)
		return ErrDialMachinePool
	}
	return nil
}

// reconnectLoop handles exponential backoff reconnection when a connection drops.
func (c *Connection) reconnectLoop() {
	backoff := 500 * time.Millisecond
	for {
		if c.ctx.Err() != nil || c.State() != protocol.ConnectionConnecting {
			return
		}
		if err := c.Connect(); err != nil {
			// Exponential backoff
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > 10*time.Second {
					backoff = 10 * time.Second
				}
			}
			continue
		}
		return
	}
}

func (c *Connection) OnMessage(socket *websocket.Conn, messageType websocket.MessageType, data []byte) {
	if messageType != websocket.BinaryMessage || len(data) < protocol.SessionKeySize || c.dispatcher == nil {
		return
	}
	key := protocol.SessionKey(data[:protocol.SessionKeySize])
	c.dispatcher(key, data[protocol.SessionKeySize:])
}

func (c *Connection) OnClose(socket *websocket.Conn, err error) {
	// Ignore if already closing or closed
	if !c.state.CompareAndSwap(int32(protocol.ConnectionActive), int32(protocol.ConnectionConnecting)) {
		return
	}
	c.socket.Store(nil)
	go c.reconnectLoop()
}

func (c *Connection) OnOpen(socket *websocket.Conn) {
	slog.Info("gateway machine pool connection opened", "machineID", c.machineID, "slot", c.slot)
	c.state.Store(int32(protocol.ConnectionActive))
	c.socket.Store(socket)
}

func (c *Connection) Close() {
	if c.State() == protocol.ConnectionClosed {
		return
	}
	c.state.Store(int32(protocol.ConnectionClosed))
	socket := c.socket.Load()
	if socket != nil {
		if err := socket.Close(); err != nil {

		}
		c.socket.Store(nil)
	}
}

func (c *Connection) WritePacket(key protocol.SessionKey, packet *protocol.Packet) error {
	return c.write(key, packet.Header(), packet.Payload)
}

func (c *Connection) Write(key protocol.SessionKey, data []byte) error {
	return c.write(key, data)
}

func (c *Connection) write(key protocol.SessionKey, bs ...[]byte) error {
	socket := c.socket.Load()
	if c.State() != protocol.ConnectionActive || socket == nil {
		return ErrConnectionNotActive
	}
	if len(bs) == 0 {
		return nil
	}
	buffer := bufferPool.Acquire()
	defer bufferPool.Release(buffer)
	required := protocol.SessionKeySize
	for _, b := range bs {
		required += len(b)
	}
	if cap(buffer.Buf) < required {
		buffer.Buf = make([]byte, required)
	} else {
		buffer.Buf = buffer.Buf[:required]
	}
	offset := copy(buffer.Buf[:protocol.SessionKeySize], key[:])
	for _, b := range bs {
		offset += copy(buffer.Buf[offset:], b)
	}
	return socket.WriteMessage(websocket.BinaryMessage, buffer.Buf)
}

func (c *Connection) State() protocol.ConnectionState {
	return protocol.ConnectionState(c.state.Load())
}
