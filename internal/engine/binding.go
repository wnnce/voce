package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

// PacketWriter is created once per connection and invoked by Session's output
// loop. It is never allocated per packet.
type PacketWriter func(*protocol.Packet) error

// WritePacket lets the session output loop invoke a PacketWriter stored behind
// an interface-free holder. A non-nil error means this frame was not written;
// it does not mean the connection is gone, so the loop keeps running.
func (o PacketWriter) WritePacket(packet *protocol.Packet) error { return o(packet) }

type Lease uint64

const (
	idleLease     Lease = 0
	stoppingLease Lease = ^idleLease
)

var (
	ErrSessionBusy       = errors.New("session is busy")
	ErrLeaseInvalid      = errors.New("session lease is invalid")
	ErrSessionCloseInput = errors.New("session close requested")
)

type sessionBinding struct {
	mu       sync.Mutex // Bind/SetWriter/Unbind only; never used by ReceivePacket.
	owner    atomic.Uint64
	sequence atomic.Uint64
	writer   atomic.Pointer[packetWriterHolder]

	outCancel context.CancelFunc
	outDone   chan struct{}
}

type packetWriterHolder struct{ writer PacketWriter }

func (s *Session) Bind() (Lease, error) {
	s.binding.mu.Lock()
	defer s.binding.mu.Unlock()
	if Lease(s.binding.owner.Load()) != idleLease {
		return 0, ErrSessionBusy
	}
	lease := Lease(s.binding.sequence.Add(1))
	if lease == idleLease || lease == stoppingLease {
		return 0, ErrSessionBusy
	}
	s.binding.owner.Store(uint64(lease))
	return lease, nil
}

// SetWriter installs the writer and is the only operation that starts outLoop.
func (s *Session) SetWriter(lease Lease, writer PacketWriter) bool {
	if writer == nil {
		return false
	}
	s.binding.mu.Lock()
	defer s.binding.mu.Unlock()
	if Lease(s.binding.owner.Load()) != lease || s.binding.outCancel != nil {
		return false
	}
	s.binding.writer.Store(&packetWriterHolder{writer: writer})
	if s.Workflow != nil {
		ctx, cancel := context.WithCancel(s.Workflow.Context())
		done := make(chan struct{})
		s.binding.outCancel = cancel
		s.binding.outDone = done
		go s.outLoop(ctx, done)
		slog.DebugContext(ctx, "session output loop started", "lease", uint64(lease))
	}
	return true
}

func (s *Session) Activate(lease Lease) error {
	s.binding.mu.Lock()
	defer s.binding.mu.Unlock()
	if Lease(s.binding.owner.Load()) != lease {
		return ErrLeaseInvalid
	}
	if s.Workflow == nil || s.Workflow.State() != WorkflowStatePaused {
		return nil
	}
	return s.Workflow.Resume()
}

// ReceivePacket performs one atomic owner check and then dispatches the packet
// input. Unbind may race with an already accepted call; that call is allowed to
// finish, while calls starting after owner changes are rejected.
// ReceivePacket borrows packet and payload only for this synchronous call. Callers
// retain ownership and must ReleasePacket after Inbound returns.
func (s *Session) ReceivePacket(lease Lease, packet *protocol.Packet) error {
	if Lease(s.binding.owner.Load()) != lease {
		return ErrLeaseInvalid
	}
	if packet == nil {
		return errors.New("session packet is nil")
	}
	s.UpdateActivity()
	switch packet.Type {
	case protocol.TypeAudio:
		if s.Workflow == nil {
			return errors.New("session workflow is nil")
		}
		audio := schema.NewAudio("audio", AudioSampleRate, AudioChannels)
		audio.SetBytes(packet.Payload)
		if err := s.Workflow.SendToHead(audio.ReadOnly()); err != nil {
			audio.Release()
			return err
		}
		return nil
	case protocol.TypeClose:
		slog.InfoContext(s.Workflow.Context(), "client requested session close")
		return ErrSessionCloseInput
	case protocol.TypePause:
		if s.Workflow == nil {
			return errors.New("session workflow is nil")
		}
		return s.Workflow.Pause()
	case protocol.TypeResume:
		if s.Workflow == nil {
			return errors.New("session workflow is nil")
		}
		return s.Workflow.Resume()
	default:
		return errors.New("unknown session packet type")
	}
}

// Unbind marks the lease stopping before waiting for outLoop. The stopping
// value keeps Bind from handing the Session to a new connection early.
func (s *Session) Unbind(lease Lease, pause bool) bool {
	s.binding.mu.Lock()
	if !s.binding.owner.CompareAndSwap(uint64(lease), uint64(stoppingLease)) {
		s.binding.mu.Unlock()
		return false
	}
	s.binding.writer.Store(nil)
	cancel, done := s.binding.outCancel, s.binding.outDone
	s.binding.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	// done is nil when the lease was bound but no writer was ever installed
	// (for example a failed WebSocket Upgrade). Receiving from a nil channel
	// blocks forever, so the wait must be conditional.
	if done != nil {
		<-done
	}
	if pause && s.Workflow != nil && s.Workflow.State() == WorkflowStateRunning {
		if err := s.Workflow.Pause(); err != nil {
			slog.ErrorContext(s.Workflow.Context(), "pause workflow on unbind failed", "error", err)
		}
	}

	s.binding.mu.Lock()
	defer s.binding.mu.Unlock()
	if s.binding.owner.Load() != uint64(stoppingLease) {
		return false
	}
	s.binding.writer.Store(nil)
	s.binding.outCancel = nil
	s.binding.outDone = nil
	s.binding.owner.Store(uint64(idleLease))
	return true
}

func (s *Session) outLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	if s.Workflow == nil {
		slog.WarnContext(ctx, "session output loop has no workflow")
		return
	}
	defer slog.DebugContext(ctx, "session output loop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok := <-s.Workflow.Output():
			if !ok {
				slog.InfoContext(ctx, "workflow output closed, stopping session output loop")
				return
			}
			if holder := s.binding.writer.Load(); holder != nil && holder.writer != nil {
				// A single failed write must not stop the loop: the connection
				// is released through Unbind, which is the only place that
				// decides to tear the session's outbound down.
				if err := holder.writer.WritePacket(packet); err != nil {
					slog.ErrorContext(ctx, "session outbound write failed",
						"type", packet.Type, "size", len(packet.Payload), "error", err)
				}
			}
			protocol.ReleasePacket(packet)
		}
	}
}

func (s *Session) closeWriter() {
	lease := Lease(s.binding.owner.Load())
	if lease != idleLease && lease != stoppingLease {
		slog.DebugContext(s.Workflow.Context(), "releasing session outbound", "lease", uint64(lease))
		_ = s.Unbind(lease, false)
	}
}
