package realtime

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

var (
	// Global counters for real-time monitoring and telemetry
	activeConnections atomic.Int64
	audioTrafficIn    atomic.Uint64
	audioTrafficOut   atomic.Uint64
)

func GetMonitorCounters() (conn int64, in uint64, out uint64) {
	return activeConnections.Load(),
		audioTrafficIn.Load(),
		audioTrafficOut.Load()
}

// SocketHandler manages the WebSocket connection and synchronizes its state with the workflow session.
type SocketHandler struct {
	gws.BuiltinEventHandler
	ctx     context.Context
	cancel  context.CancelFunc
	socket  *gws.Conn
	session *engine.Session
	running atomic.Bool
	held    bool
}

func NewSocketHandler(session *engine.Session) *SocketHandler {
	sessionCtx, cancel := context.WithCancel(session.Workflow.Context())
	return &SocketHandler{
		ctx:     sessionCtx,
		cancel:  cancel,
		session: session,
	}
}

func (s *SocketHandler) OnOpen(socket *gws.Conn) {
	s.socket = socket

	if !s.session.Acquire() {
		slog.WarnContext(s.ctx, "session is busy, closing connection", "sessionID", s.session.ID)
		_ = socket.WriteClose(1008, []byte("session is busy"))
		return
	}
	s.held = true

	s.running.Store(true)

	activeConnections.Add(1)

	// Resume the workflow if it was paused from a previous session (Persistence Support)
	if s.session.Workflow.State() == engine.WorkflowStatePaused {
		if err := s.session.Workflow.Resume(); err != nil {
			slog.ErrorContext(s.ctx, "resume workflow failed", "error", err)
		}
	}
	go s.writeLoop()
	slog.InfoContext(s.ctx, "socket connected")
}

func (s *SocketHandler) OnClose(_ *gws.Conn, _ error) {
	s.running.Store(false)

	activeConnections.Add(-1)

	if s.held {
		s.session.Release()
	}

	if s.cancel != nil {
		s.cancel()
	}

	// Pause instead of Stop to allow for 'Persistent Sessions' where a client can reconnect later.
	if s.session.Workflow.State() == engine.WorkflowStateRunning {
		if err := s.session.Workflow.Pause(); err != nil {
			slog.ErrorContext(s.ctx, "socket close, pause workflow failed", "error", err)
		}
	}
	slog.InfoContext(s.ctx, "socket disconnected")
}

func (s *SocketHandler) OnPing(socket *gws.Conn, _ []byte) {
	s.session.UpdateActivity()
	_ = socket.WritePong(nil)
}

// OnMessage handles incoming packets from the client and routes them to the workflow engine.
func (s *SocketHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	body := message.Bytes()
	defer message.Close()
	if message.Opcode != gws.OpcodeBinary {
		return
	}
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	if err := packet.Unmarshal(body); err != nil {
		slog.WarnContext(s.ctx, "packet unmarshal failed", "error", err)
		return
	}

	audioTrafficIn.Add(uint64(len(body)))

	switch packet.Type {
	case protocol.TypeAudio:
		audio := schema.NewAudio("audio", engine.AudioSampleRate, engine.AudioChannels)
		audio.SetBytes(packet.Payload)
		if err := s.session.Workflow.SendToHead(audio.ReadOnly()); err != nil {
			slog.ErrorContext(s.ctx, "send audio to workflow failed", "error", err)
			audio.Release()
		}
	case protocol.TypeClose:
		slog.InfoContext(s.ctx, "received close packet, stopping session")
		engine.DefaultSessionManager.RemoveSession(s.session.ID)
		if err := s.socket.WriteClose(1000, nil); err != nil {
			slog.ErrorContext(s.ctx, "close socket failed", "error", err)
		}
	}
}

// writeLoop serializes workflow output packets into the WebSocket connection.
func (s *SocketHandler) writeLoop() {
	defer func() {
		if !s.running.Load() {
			return
		}
		if err := s.socket.WriteClose(1000, nil); err != nil {
			slog.ErrorContext(s.ctx, "close socket failed", "error", err)
		}
	}()
	for {
		if s.ctx.Err() != nil || !s.running.Load() {
			return
		}
		select {
		case <-s.ctx.Done():
			return
		case packet, ok := <-s.session.Workflow.Output():
			if !ok {
				return
			}
			header := packet.Header()
			payload := packet.Payload

			audioTrafficOut.Add(uint64(len(header) + len(payload)))

			slog.DebugContext(s.ctx, "Send packet to client", "type", packet.Type, "size", len(payload))
			if err := s.socket.Writev(gws.OpcodeBinary, header, payload); err != nil {
				slog.ErrorContext(s.ctx, "websocket write binary packet failed", "error", err)
			}
			protocol.ReleasePacket(packet)
		}
	}
}
