package realtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	pb "github.com/wnnce/voce/api/voce/v1"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type StreamService struct {
	pb.UnimplementedVoceServiceServer
	manager *engine.SessionManager
}

func NewStreamService(manager *engine.SessionManager) *StreamService {
	return &StreamService{manager: manager}
}

func (s *StreamService) RealtimeStream(stream pb.VoceService_RealtimeStreamServer) error {
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok || len(md.Get("session_id")) == 0 {
		return status.Error(codes.InvalidArgument, "missing session_id in metadata")
	}
	sessionId := md.Get("session_id")[0]
	sessionKey, err := protocol.ParseSessionKey(sessionId)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid session_id")
	}

	session, ok := s.manager.LoadSession(sessionKey)
	if !ok {
		return status.Errorf(codes.NotFound, "session %s not found", sessionId)
	}

	if !session.Acquire() {
		return status.Errorf(codes.ResourceExhausted, "session %s is busy", sessionId)
	}
	defer func() {
		session.Release()
		if session.Workflow.State() == engine.WorkflowStateRunning {
			_ = session.Workflow.Pause()
			slog.InfoContext(stream.Context(), "grpc disconnected, session paused", "id", sessionId)
		}
	}()

	if session.Workflow.State() == engine.WorkflowStatePaused {
		if err := session.Workflow.Resume(); err != nil {
			slog.ErrorContext(stream.Context(), "resume workflow failed", "error", err)
			return status.Errorf(codes.Internal, "resume failed: %v", err)
		}
	}
	slog.InfoContext(stream.Context(), "grpc session connected", "id", sessionId)

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go s.writeLoop(ctx, cancel, stream, session, wg)
	go s.readLoop(ctx, cancel, stream, session, wg)
	wg.Wait()

	slog.InfoContext(stream.Context(), "grpc session closed", "id", sessionId)
	return nil
}

func (s *StreamService) writeLoop(
	ctx context.Context,
	cancel context.CancelFunc,
	stream pb.VoceService_RealtimeStreamServer,
	session *engine.Session,
	wg *sync.WaitGroup,
) {
	defer func() {
		cancel()
		wg.Done()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok := <-session.Workflow.Output():
			if !ok {
				slog.WarnContext(ctx, "workflow output channel closed")
				return
			}
			resp := &pb.Packet{
				Type:    pb.PacketType(packet.Type),
				Encode:  pb.PacketEncode(packet.Encode),
				Payload: packet.Payload,
			}
			if err := stream.Send(resp); err != nil {
				slog.ErrorContext(ctx, "grpc send failed", "error", err)
				return
			}
			audioTrafficOut.Add(uint64(len(packet.Payload)))
			protocol.ReleasePacket(packet)
		}
	}
}

func (s *StreamService) readLoop(
	ctx context.Context,
	cancel context.CancelFunc,
	stream pb.VoceService_RealtimeStreamServer,
	session *engine.Session,
	wg *sync.WaitGroup,
) {
	defer func() {
		cancel()
		wg.Done()
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		req, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.ErrorContext(ctx, "grpc recv failed", "error", err)
			}
			return
		}
		session.UpdateActivity()
		switch protocol.PacketType(req.Type) {
		case protocol.TypeAudio:
			audio := schema.NewAudio("audio", engine.AudioSampleRate, engine.AudioChannels)
			audio.SetBytes(req.Payload)
			audioTrafficIn.Add(uint64(len(req.Payload)))

			if err = session.Workflow.SendToHead(audio.ReadOnly()); err != nil {
				audio.Release()
				slog.ErrorContext(ctx, "send to workflow failed", "error", err)
			}
		case protocol.TypeClose:
			slog.InfoContext(ctx, "client requested close via packet")
			s.manager.RemoveSession(session.Key)
			return
		}
	}
}
