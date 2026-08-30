package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/buf"
	"github.com/wnnce/voce/pkg/pool"
	"github.com/wnnce/voce/pkg/retry"
)

type MessageDispatcher func(key protocol.SessionKey, data []byte)

// ConnectionObserver receives data-plane connection lifecycle changes.
type ConnectionObserver interface {
	OnConnectionOpen(*Connection)
	OnConnectionClose(*Connection)
}

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
	observer   ConnectionObserver
}

func newPoolConnection(
	ctx context.Context,
	engine *nbhttp.Engine,
	machineID, address string,
	dispatcher MessageDispatcher,
	observer ConnectionObserver,
) (*Connection, error) {
	u, err := url.Parse("ws://" + address + "/pool")
	if err != nil {
		return nil, ErrInvalidPoolAddress
	}
	conn := &Connection{
		machineID:  machineID,
		ctx:        ctx,
		addr:       u,
		dispatcher: dispatcher,
		observer:   observer,
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
	return conn, nil
}

// Connect initiates the WebSocket handshake.
func (c *Connection) Connect() error {
	gatewayPoolMetrics.dials.Add(gatewayPoolMetricContext, 1)
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
	backoff := retry.NewBackoff(500*time.Millisecond, 10*time.Second)
	for {
		if c.ctx.Err() != nil || c.State() != protocol.ConnectionConnecting {
			return
		}
		if err := c.Connect(); err != nil {
			if err := backoff.Wait(c.ctx); err != nil {
				return
			}
			continue
		}
		return
	}
}

func (c *Connection) OnMessage(socket *websocket.Conn, messageType websocket.MessageType, data []byte) {
	if messageType != websocket.BinaryMessage || len(data) < protocol.SessionKeySize {
		return
	}
	gatewayPoolMetrics.bytesReceived.Add(gatewayPoolMetricContext, int64(len(data)))
	gatewayPoolMetrics.packetsReceived.Add(gatewayPoolMetricContext, 1)
	if c.dispatcher == nil {
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
	if c.observer != nil {
		c.observer.OnConnectionClose(c)
	}
	go c.reconnectLoop()
}

func (c *Connection) OnOpen(socket *websocket.Conn) {
	slog.Info("gateway machine pool connection opened", "machineID", c.machineID)
	if !c.state.CompareAndSwap(int32(protocol.ConnectionConnecting), int32(protocol.ConnectionActive)) {
		_ = socket.Close()
		return
	}
	c.socket.Store(socket)
	if c.observer != nil {
		c.observer.OnConnectionOpen(c)
	}
}

func (c *Connection) Close() {
	if protocol.ConnectionState(c.state.Swap(int32(protocol.ConnectionClosed))) == protocol.ConnectionClosed {
		return
	}
	socket := c.socket.Load()
	if socket != nil {
		_ = socket.Close()
		c.socket.Store(nil)
	}
	if c.observer != nil {
		c.observer.OnConnectionClose(c)
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
		gatewayPoolMetrics.writeErrors.Add(gatewayPoolMetricContext, 1)
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
	if err := socket.WriteMessage(websocket.BinaryMessage, buffer.Buf); err != nil {
		gatewayPoolMetrics.writeErrors.Add(gatewayPoolMetricContext, 1)
		return err
	}
	gatewayPoolMetrics.bytesSent.Add(gatewayPoolMetricContext, int64(len(buffer.Buf)))
	gatewayPoolMetrics.packetsSent.Add(gatewayPoolMetricContext, 1)
	return nil
}

func (c *Connection) State() protocol.ConnectionState {
	return protocol.ConnectionState(c.state.Load())
}
