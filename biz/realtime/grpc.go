package realtime

import (
	"context"
	"errors"
	"io"
	"log/slog"

	pb "github.com/wnnce/voce/api/voce/v1"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/internal/protocol"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
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
	md, ok := grpcmetadata.FromIncomingContext(stream.Context())
	if !ok || len(md.Get("session_id")) == 0 {
		return status.Error(codes.InvalidArgument, "missing session_id in metadata")
	}
	sessionID := md.Get("session_id")[0]
	sessionKey, err := protocol.ParseSessionKey(sessionID)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid session_id")
	}

	session, ok := s.manager.LoadSession(sessionKey)
	if !ok {
		return status.Errorf(codes.NotFound, "session %s not found", sessionID)
	}

	// Carry the session into the logger's context group, matching the HTTP path
	// where the same value is stored under trace_id at session creation.
	ctx := context.WithValue(stream.Context(), metadata.ContextTraceKey, sessionID)

	lease, err := session.Bind()
	if err != nil {
		return status.Errorf(codes.ResourceExhausted, "session %s is busy", sessionID)
	}
	defer func() {
		_ = session.Unbind(lease, true)
		slog.InfoContext(ctx, "grpc session released")
	}()

	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !session.SetWriter(lease, s.withPacketWriter(stream, cancel)) {
		slog.WarnContext(ctx, "grpc session writer mount failed", "lease", uint64(lease))
		return status.Error(codes.Aborted, "session lease mount failed")
	}
	if err := session.Activate(lease); err != nil {
		slog.ErrorContext(ctx, "grpc activate workflow failed", "error", err)
		return status.Errorf(codes.Internal, "resume failed: %v", err)
	}
	slog.InfoContext(ctx, "grpc session connected")

	go s.readLoop(connectionCtx, stream, session, lease, cancel)
	<-connectionCtx.Done()
	slog.InfoContext(ctx, "grpc session closed")
	return nil
}

// readLoop is the single synchronous Recv path. It shares the connection
// context with the Session output loop; any terminal read/input result cancels
// the same connection lifecycle.
func (s *StreamService) readLoop(
	ctx context.Context,
	stream pb.VoceService_RealtimeStreamServer,
	session *engine.Session,
	lease engine.Lease,
	cancel context.CancelFunc,
) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		req, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.ErrorContext(ctx, "grpc recv failed", "error", err)
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		realtimeMetrics.grpcBytesReceived.Add(realtimeMetricContext, int64(len(req.Payload)))
		packet := protocol.AcquirePacket()
		packet.Type = protocol.PacketType(req.Type)
		packet.Encode = protocol.PacketEncode(req.Encode)
		packet.SetPayload(req.Payload)
		err = session.ReceivePacket(lease, packet)
		protocol.ReleasePacket(packet)
		if errors.Is(err, engine.ErrSessionCloseInput) {
			s.manager.RemoveSession(session.Key)
			return
		}
		if errors.Is(err, engine.ErrLeaseInvalid) {
			return
		}
		if err != nil {
			slog.ErrorContext(ctx, "grpc session input failed", "type", packet.Type, "error", err)
			return
		}
	}
}

func (s *StreamService) withPacketWriter(
	stream pb.VoceService_RealtimeStreamServer,
	cancel context.CancelFunc,
) engine.PacketWriter {
	return func(packet *protocol.Packet) error {
		if err := stream.Send(&pb.Packet{
			Type:    pb.PacketType(packet.Type),
			Encode:  pb.PacketEncode(packet.Encode),
			Payload: packet.Payload,
		}); err != nil {
			if cancel != nil {
				cancel()
			}
			return err
		}
		realtimeMetrics.grpcBytesSent.Add(realtimeMetricContext, int64(len(packet.Payload)))
		return nil
	}
}
