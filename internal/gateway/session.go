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

type SessionState int32

const (
	SessionIdle SessionState = iota + 1
	SessionPending
	SessionReady
	SessionClosing
	SessionClosed
)

type Session struct {
	key          protocol.SessionKey
	conn         *Connection
	machine      *Machine
	client       *websocket.Conn
	createdAt    time.Time
	lastActiveAt atomic.Int64
	state        atomic.Int32
	connectCount atomic.Int32
}

func gatewayPacketTypeName(t protocol.PacketType) string {
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

func NewSession(key protocol.SessionKey, conn *Connection, machine *Machine) *Session {
	s := &Session{
		key:       key,
		conn:      conn,
		machine:   machine,
		createdAt: time.Now(),
	}
	s.lastActiveAt.Store(time.Now().UnixMilli())
	return s
}

func (s *Session) State() SessionState {
	return SessionState(s.state.Load())
}

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

func (s *Session) Close() {
	if s.State() == SessionClosed {
		return
	}
	s.state.Store(int32(SessionClosed))

	if s.client != nil {
		_ = s.client.Close()
	}

	if s.conn == nil {
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypeClose
	if err := s.conn.WritePacket(s.key, packet); err != nil && !errors.Is(err, ErrConnectionNotActive) {
		slog.Error("failed to write close packet", "error", err)
	}
}

func (s *Session) OnClientOpen(socket *websocket.Conn) {
	count := s.connectCount.Add(1)
	slog.Info("client connected", "session", s.key, "count", count, "remote", socket.RemoteAddr())

	s.state.Store(int32(SessionReady))
	s.lastActiveAt.Store(time.Now().UnixMilli())
	s.client = socket

	// 如果 count > 1，说明是异常断开后的“恢复”连接，需要通知后端 Resume
	if count > 1 && s.conn != nil {
		packet := protocol.AcquirePacket()
		defer protocol.ReleasePacket(packet)
		packet.Type = protocol.TypeResume
		if err := s.conn.WritePacket(s.key, packet); err != nil {
			slog.Error("failed to write resume packet", "error", err, "session", s.key)
		}
	}
}

func (s *Session) OnClientClose(socket *websocket.Conn, err error) {
	slog.Warn("client disconnected", "session", s.key, "error", err)
	s.client = nil
	if s.State() == SessionClosed {
		return
	}
	s.Release()
	if s.conn == nil {
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypePause
	if err = s.conn.WritePacket(s.key, packet); err != nil {
		slog.Error("failed to write pause packet", "error", err, "session", s.key)
	}
}

func (s *Session) OnClientPing(socket *websocket.Conn, payload string) {
	s.lastActiveAt.Store(time.Now().UnixMilli())
	if err := socket.WriteMessage(websocket.PongMessage, nil); err != nil {
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
	if s.machine.State() != MachineStateActive || s.conn.State() != protocol.ConnectionActive {
		// Log at Debug level to avoid log storm during pod suspension/reconnection
		slog.Debug("gateway skipped forwarding client packet because upstream is not ready",
			"session", s.key,
			"machineState", s.machine.State(),
			"connectionState", s.conn.State(),
		)
		return
	}
	s.lastActiveAt.Store(time.Now().UnixMilli())
	if err := s.conn.Write(s.key, data); err != nil {
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
		m.shards.Delete(s.key)
		s.Close()
		if s.machine != nil {
			s.machine.RemoveSession(s.key)
		}
	}
}

func (m *SessionManager) Store(s *Session) {
	m.shards.Store(s.key, s)
}

func (m *SessionManager) Load(key protocol.SessionKey) (*Session, bool) {
	return m.shards.Load(key)
}

func (m *SessionManager) Delete(key protocol.SessionKey) {
	if s, ok := m.shards.Load(key); ok {
		s.Close()
		if s.machine != nil {
			s.machine.RemoveSession(s.key)
		}
	}
	m.shards.Delete(key)
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
		// If the pod notifies the gateway to close the session, we explicitly close the client
		// connection and update the state to SessionClosed. This prevents m.Delete(key) from
		// triggering s.Close(), which would send an unnecessary loopback Close packet back to the pod.
		if session.client != nil {
			_ = session.client.Close()
		}
		session.state.Store(int32(SessionClosed))
		m.Delete(key)
	} else {
		if session.State() != SessionReady || session.client == nil {
			slog.Warn("gateway skipped writing machine packet to client because client is not ready",
				"session", key,
				"state", session.State(),
				"hasClient", session.client != nil,
				"type", gatewayPacketTypeName(packet.Type),
			)
			return
		}
		if err := session.client.WriteMessage(websocket.BinaryMessage, data); err != nil {
			slog.Error("failed to write message to client", "error", err, "session", key)
		}
	}
}
