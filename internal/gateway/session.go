package gateway

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/syncx"
)

var (
	clientConnections atomic.Int64
)

type SessionState int32

const (
	SessionIdle SessionState = iota + 1
	SessionPending
	SessionReady
	SessionClosing
	SessionClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionIdle:
		return "idle"
	case SessionPending:
		return "pending"
	case SessionReady:
		return "ready"
	case SessionClosing:
		return "closing"
	case SessionClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session represents an end-to-end user conversation session.
// It maps a client WebSocket connection to a specific backend machine.
type Session struct {
	key          protocol.SessionKey
	binding      *SessionBinding
	machine      *Machine
	client       *websocket.Conn
	createdAt    time.Time
	lastActiveAt atomic.Int64
	state        atomic.Int32
	connectCount atomic.Int32
}

// NewSession creates a new session mapping for the given key.
func NewSession(key protocol.SessionKey, binding *SessionBinding, machine *Machine) *Session {
	s := &Session{
		key:       key,
		binding:   binding,
		machine:   machine,
		createdAt: time.Now(),
	}
	s.state.Store(int32(SessionIdle))
	s.lastActiveAt.Store(time.Now().UnixMilli())
	return s
}

func (s *Session) Connection() *Connection {
	if s.binding == nil {
		return nil
	}
	return s.binding.Connection()
}

func (s *Session) State() SessionState {
	return SessionState(s.state.Load())
}

// Acquire attempts to transition the session from Idle to Pending.
func (s *Session) Acquire() bool {
	return s.state.CompareAndSwap(int32(SessionIdle), int32(SessionPending))
}

func (s *Session) Release() bool {
	state := s.State()
	if state != SessionReady && state != SessionPending {
		return false
	}
	s.state.Store(int32(SessionIdle))
	return true
}

// Close terminates the session and notifies both client and upstream machine.
func (s *Session) Close() {
	if s.state.Swap(int32(SessionClosed)) == int32(SessionClosed) {
		return
	}

	if s.client != nil {
		_ = s.client.Close()
	}

	conn := s.Connection()
	if conn == nil {
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypeClose
	if err := conn.WritePacket(s.key, packet); err != nil && !errors.Is(err, ErrConnectionNotActive) {
		slog.Error("failed to write close packet", "error", err)
	}
}

func (s *Session) OnClientOpen(socket *websocket.Conn) {
	count := s.connectCount.Add(1)
	slog.Info("client connected", "session", s.key, "count", count, "remote", socket.RemoteAddr())

	s.state.Store(int32(SessionReady))
	s.lastActiveAt.Store(time.Now().UnixMilli())
	s.client = socket
	clientConnections.Add(1)
	gatewayClientMetrics.connectionsActive.Add(gatewayClientMetricContext, 1)

	// If count > 1, this is a reconnect. Notify the backend to resume.
	if conn := s.Connection(); count > 1 && conn != nil {
		packet := protocol.AcquirePacket()
		defer protocol.ReleasePacket(packet)
		packet.Type = protocol.TypeResume
		if err := conn.WritePacket(s.key, packet); err != nil {
			slog.Error("failed to write resume packet", "error", err, "session", s.key)
		}
	}
}

func (s *Session) OnClientClose(socket *websocket.Conn, err error) {
	slog.Warn("client disconnected", "session", s.key, "error", err)
	s.client = nil
	clientConnections.Add(-1)
	gatewayClientMetrics.connectionsActive.Add(gatewayClientMetricContext, -1)
	if s.State() == SessionClosed {
		return
	}
	s.Release()
	conn := s.Connection()
	if conn == nil {
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypePause
	if err = conn.WritePacket(s.key, packet); err != nil {
		slog.Error("failed to write pause packet", "error", err, "session", s.key)
	}
}

func (s *Session) OnClientPing(socket *websocket.Conn, payload string) {
	s.lastActiveAt.Store(time.Now().UnixMilli())
	if err := socket.WriteMessage(websocket.PongMessage, nil); err != nil {
		gatewayClientMetrics.writeErrors.Add(gatewayClientMetricContext, 1)
		slog.Error("failed to send pong to client", "error", err, "session", s.key)
	}
}

func (s *Session) OnClientMessage(_ *websocket.Conn, messageType websocket.MessageType, data []byte) {
	if messageType != websocket.BinaryMessage {
		slog.Warn("gateway dropped non-binary client message", "session", s.key, "messageType", messageType)
		return
	}
	if len(data) < protocol.PacketHeaderSize {
		slog.Warn("gateway dropped short client packet", "session", s.key, "size", len(data))
		return
	}
	if data[0] != protocol.MagicNumber1 || data[1] != protocol.MagicNumber2 {
		slog.Warn("gateway dropped invalid client packet magic", "session", s.key, "size", len(data))
		return
	}
	if binary.BigEndian.Uint32(data[4:8]) != uint32(len(data[protocol.PacketHeaderSize:])) {
		slog.Warn("gateway dropped client packet with invalid payload size", "session", s.key, "size", len(data))
		return
	}
	gatewayClientMetrics.bytesReceived.Add(gatewayClientMetricContext, int64(len(data)))
	gatewayClientMetrics.packetsReceived.Add(gatewayClientMetricContext, 1)
	conn := s.Connection()
	if s.machine.State() != MachineStateActive || conn == nil || conn.State() != protocol.ConnectionActive {
		// Log at Debug level to avoid log storm during pod suspension/reconnection
		slog.Debug("gateway skipped forwarding client packet because upstream is not ready",
			"session", s.key,
			"machineState", s.machine.State(),
			"connectionState", connectionState(conn),
		)
		return
	}
	s.lastActiveAt.Store(time.Now().UnixMilli())
	if err := conn.Write(s.key, data); err != nil {
		slog.Error("pool connection write failed", "error", err)
	}
}

type SessionManager struct {
	shards *syncx.ShardedMap[protocol.SessionKey, *Session]
}

func NewSessionManager(ctx context.Context, timeout, interval time.Duration) *SessionManager {
	m := &SessionManager{
		shards: syncx.NewShardedMap[protocol.SessionKey, *Session](64, func(key protocol.SessionKey) uint64 {
			return binary.BigEndian.Uint64(key[:8]) ^ binary.BigEndian.Uint64(key[8:])
		}),
	}
	slog.Info("session manager started", "timeout", timeout, "interval", interval)
	go m.run(ctx, timeout, interval)
	return m
}

func (m *SessionManager) run(ctx context.Context, timeout, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanup(timeout)
		}
	}
}

func (m *SessionManager) cleanup(timeout time.Duration) {
	threshold := time.Now().Add(-timeout).UnixMilli()
	var expired []*Session
	m.shards.Range(func(_ protocol.SessionKey, s *Session) bool {
		last := s.lastActiveAt.Load()
		if last > 0 && last < threshold {
			expired = append(expired, s)
		}
		return true
	})

	if len(expired) > 0 {
		slog.Info("session cleanup", "count", len(expired))
	}

	for _, s := range expired {
		m.Delete(s.key)
	}
}

func (m *SessionManager) Store(s *Session) {
	var added bool
	m.shards.Update(s.key, func(existing *Session, ok bool) (*Session, bool) {
		if !ok {
			added = true
		}
		return s, true
	})
	if !added {
		return
	}
	gatewaySessionMetrics.addActive(1)
}

func (m *SessionManager) Load(key protocol.SessionKey) (*Session, bool) {
	return m.shards.Load(key)
}

func (m *SessionManager) Delete(key protocol.SessionKey) {
	var s *Session
	removed := m.shards.Update(key, func(existing *Session, ok bool) (*Session, bool) {
		if !ok {
			return nil, false
		}
		s = existing
		return existing, false
	})
	if !removed {
		return
	}
	s.Close()
	if s.machine != nil {
		s.machine.Pool.Unbind(s.key)
		s.machine.RemoveSession(s.key)
	}
	gatewaySessionMetrics.addActive(-1)
}

func (m *SessionManager) Count() int64 {
	var count int64
	m.shards.Range(func(_ protocol.SessionKey, _ *Session) bool {
		count++
		return true
	})
	return count
}

func connectionState(conn *Connection) protocol.ConnectionState {
	if conn == nil {
		return protocol.ConnectionClosed
	}
	return conn.State()
}

func (m *SessionManager) DispatchMessage(key protocol.SessionKey, data []byte) {
	session, ok := m.Load(key)
	if !ok {
		slog.Warn("gateway dropped machine packet for missing session", "session", key, "size", len(data))
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	if err := packet.UnmarshalHeader(data); err != nil {
		slog.Warn("gateway failed to unmarshal machine packet header", "session", key, "error", err, "size", len(data))
		return
	}
	if packet.Type == protocol.TypeClose {
		// Mark the session terminal before closing the client socket. Its close callback must
		// not treat this machine-initiated termination as a reconnectable disconnect and
		// send a late Pause packet back to the machine.
		session.state.Store(int32(SessionClosed))
		if session.client != nil {
			_ = session.client.Close()
		}
		m.Delete(key)
	} else {
		if session.State() != SessionReady || session.client == nil {
			slog.Warn("gateway skipped writing machine packet to client because client is not ready",
				"session", key,
				"state", session.State(),
				"hasClient", session.client != nil,
				"type", packet.Type,
			)
			return
		}
		if err := session.client.WriteMessage(websocket.BinaryMessage, data); err != nil {
			gatewayClientMetrics.writeErrors.Add(gatewayClientMetricContext, 1)
			slog.Error("failed to write message to client", "error", err, "session", key)
		} else {
			gatewayClientMetrics.bytesSent.Add(gatewayClientMetricContext, int64(len(data)))
			gatewayClientMetrics.packetsSent.Add(gatewayClientMetricContext, 1)
		}
	}
}
