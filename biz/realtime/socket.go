package realtime

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"

	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
)

// SocketHandler owns only WebSocket protocol callbacks. Session owns the
// Workflow input/output lifecycle through its lease.
type SocketHandler struct {
	gws.BuiltinEventHandler
	ctx     context.Context
	session *engine.Session
	sm      *engine.SessionManager
	lease   engine.Lease
	socket  *gws.Conn
	active  atomic.Bool
	closed  atomic.Bool
}

func NewSocketHandler(sm *engine.SessionManager, session *engine.Session, lease engine.Lease) *SocketHandler {
	return &SocketHandler{
		ctx:     session.Workflow.Context(),
		session: session,
		sm:      sm,
		lease:   lease,
	}
}

func (s *SocketHandler) OnOpen(socket *gws.Conn) {
	s.socket = socket
	if !s.session.SetWriter(s.lease, s.withPacketWrite(socket)) {
		slog.WarnContext(s.ctx, "session writer mount failed",
			"lease", uint64(s.lease),
			"state", s.session.Workflow.State().String())
		_ = socket.WriteClose(1008, []byte("session is unavailable"))
		return
	}
	if err := s.session.Activate(s.lease); err != nil {
		slog.ErrorContext(s.ctx, "activate workflow failed", "error", err)
		_ = s.session.Unbind(s.lease, false)
		_ = socket.WriteClose(1011, []byte("workflow unavailable"))
		return
	}
	s.active.Store(true)
	slog.InfoContext(s.ctx, "socket connected")
}

func (s *SocketHandler) OnClose(_ *gws.Conn, _ error) {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	_ = s.session.Unbind(s.lease, s.active.Load())
	s.active.Store(false)
	slog.InfoContext(s.ctx, "socket disconnected")
}

func (s *SocketHandler) OnPing(socket *gws.Conn, _ []byte) {
	s.session.UpdateActivity()
	_ = socket.WritePong(nil)
}

func (s *SocketHandler) OnMessage(_ *gws.Conn, message *gws.Message) {
	body := message.Bytes()
	defer message.Close()
	if message.Opcode != gws.OpcodeBinary {
		return
	}
	if !s.active.Load() {
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	if err := packet.Unmarshal(body); err != nil {
		slog.WarnContext(s.ctx, "packet unmarshal failed", "error", err)
		return
	}

	realtimeMetrics.websocketBytesReceived.Add(realtimeMetricContext, int64(len(body)))
	err := s.session.ReceivePacket(s.lease, packet)
	if errors.Is(err, engine.ErrSessionCloseInput) {
		s.sm.RemoveSession(s.session.Key)
		if closeErr := s.socket.WriteClose(1000, nil); closeErr != nil {
			slog.ErrorContext(s.ctx, "close socket failed",
				"error", closeErr)
		}
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, engine.ErrLeaseInvalid) {
		slog.ErrorContext(s.ctx, "websocket input handling failed",
			"type", packet.Type, "error", err)
	}
}

func (s *SocketHandler) withPacketWrite(socket *gws.Conn) engine.PacketWriter {
	return func(packet *protocol.Packet) error {
		header := packet.Header()
		payload := packet.Payload

		realtimeMetrics.websocketBytesSent.Add(realtimeMetricContext, int64(len(header)+len(payload)))
		slog.DebugContext(s.ctx, "send packet to client",
			"type", packet.Type, "size", len(payload))

		return socket.Writev(gws.OpcodeBinary, header, payload)
	}
}
