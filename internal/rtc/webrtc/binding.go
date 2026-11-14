package webrtc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
	"github.com/wnnce/voce/pkg/buf"
)

const (
	// audioBlockSize is one second of 16k mono PCM16, the shared pool block size.
	audioBlockSize           = engine.AudioFrameSize * 100
	disconnectGrace          = 15 * time.Second
	maxDataChannelPacketSize = 64 * 1024
)

// Binding adapts one WebRTC media session to one Session. It receives the client's
// Opus audio, decodes and aggregates it into Session-sized frames, and owns the connection
// together with the session lease that authorizes it.
//
// Callbacks are installed by New and may run concurrently.
type Binding struct {
	session       *engine.Session
	lease         engine.Lease
	cred          any
	media         *media.Session
	provider      Provider
	handle        string
	encoder       *media.AudioEncoder
	onClosed      func()
	removeSession func()

	ctx    context.Context
	cancel context.CancelFunc

	connected       atomic.Bool
	closed          atomic.Bool
	running         atomic.Bool // the sender loop runs at most once at a time
	dcReady         atomic.Bool // datachannel open
	audioTrackSeen  atomic.Bool
	dataChannel     atomic.Pointer[pionwebrtc.DataChannel]
	disconnectTimer atomic.Pointer[time.Timer]

	queue *buf.ChunkQueue
	frame []byte
}

func newBinding(
	session *engine.Session,
	lease engine.Lease,
	cred any,
	mediaSession *media.Session,
	provider Provider,
	handle string,
	pool *buf.ChunkPool,
	encoder *media.AudioEncoder,
	onClosed func(),
	removeSession func(),
) *Binding {
	ctx, cancel := context.WithCancel(session.Workflow.Context())
	binding := &Binding{
		session:       session,
		lease:         lease,
		cred:          cred,
		media:         mediaSession,
		provider:      provider,
		handle:        handle,
		encoder:       encoder,
		onClosed:      onClosed,
		removeSession: removeSession,
		ctx:           ctx,
		cancel:        cancel,
		queue:         buf.NewChunkQueue(pool, 0),
		frame:         make([]byte, media.PCMFrameSize),
	}

	if mediaSession != nil && mediaSession.PC() != nil {
		mediaSession.PC().OnConnectionStateChange(binding.onConnectionStateChange)
		mediaSession.PC().OnTrack(binding.onTrack)
		mediaSession.PC().OnDataChannel(binding.onDataChannel)
		binding.setDataChannel(mediaSession.DataChannel())
	}

	return binding
}

func (b *Binding) setDataChannel(dc *pionwebrtc.DataChannel) {
	if dc == nil {
		return
	}
	b.dataChannel.Store(dc)

	dc.OnOpen(func() {
		b.dcReady.Store(true)
		slog.InfoContext(b.ctx, "webrtc datachannel opened")
	})
	dc.OnClose(func() {
		b.dcReady.Store(false)
		slog.InfoContext(b.ctx, "webrtc datachannel closed")
	})
	dc.OnMessage(b.onDataChannelMessage)
}

func (b *Binding) onDataChannel(dc *pionwebrtc.DataChannel) {
	slog.InfoContext(b.ctx, "webrtc remote datachannel received", "label", dc.Label())
	b.setDataChannel(dc)
}

// Write is the session's PacketWriter: it copies outbound audio into the queue
// and drops pending audio when an interrupt arrives. Non-audio packets are sent via DataChannel.
// It never blocks.
func (b *Binding) Write(packet *protocol.Packet) error {
	switch packet.Type {
	case protocol.TypeAudio:
		audio := packet.Payload
		if _, err := b.queue.Write(audio); err != nil {
			slog.WarnContext(b.ctx, "webrtc outbound audio dropped", "size", len(audio), "error", err)
		}
	case protocol.TypePause, protocol.TypeResume:
		// Server-to-client control packets are driven by transport state.
	case protocol.TypeInterrupter:
		b.queue.Reset()
		b.sendDataChannelMessage(packet)
	default:
		b.sendDataChannelMessage(packet)
	}
	return nil
}

func (b *Binding) sendDataChannelMessage(packet *protocol.Packet) {
	if !b.dcReady.Load() {
		return
	}
	dc := b.dataChannel.Load()
	if dc == nil || dc.ReadyState() != pionwebrtc.DataChannelStateOpen {
		return
	}

	marshaled := packet.Marshal()
	if err := dc.Send(marshaled); err != nil {
		slog.WarnContext(b.ctx, "webrtc datachannel send failed", "type", packet.Type, "error", err)
	}
}

// sendLoop paces queued audio, one 20ms frame per tick.
func (b *Binding) sendLoop() {
	if b.closed.Load() || !b.running.CompareAndSwap(false, true) {
		return
	}
	ticker := time.NewTicker(media.PacingInterval)
	defer func() {
		b.running.Store(false)
		ticker.Stop()
	}()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
		}

		if b.closed.Load() || b.ctx.Err() != nil {
			return
		}
		if !b.connected.Load() {
			continue
		}

		read, err := b.queue.Read(b.frame)
		if read == 0 {
			if err != nil && !errors.Is(err, io.EOF) {
				slog.WarnContext(b.ctx, "webrtc outbound audio read failed", "error", err)
			}
			continue
		}

		encoded, err := b.encoder.Encode(b.frame[:read])
		if err != nil {
			slog.WarnContext(b.ctx, "webrtc opus encode failed", "error", err)
			continue
		}

		if b.media == nil || b.media.AudioTrack() == nil {
			continue
		}

		if err = b.media.AudioTrack().WriteSample(pionmedia.Sample{
			Data:     encoded,
			Duration: media.PacingInterval,
		}); err != nil {
			slog.WarnContext(b.ctx, "webrtc audio track write sample failed", "error", err)
		}
	}
}

func (b *Binding) onTrack(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
	slog.InfoContext(b.ctx, "webrtc received remote track",
		"kind", track.Kind().String(),
		"id", track.ID(),
		"mimeType", track.Codec().MimeType,
	)
	if track.Kind() == pionwebrtc.RTPCodecTypeAudio && b.audioTrackSeen.CompareAndSwap(false, true) {
		b.startAudioTrackWorker(track)
	} else if track.Kind() == pionwebrtc.RTPCodecTypeAudio {
		slog.WarnContext(b.ctx, "webrtc ignoring additional remote audio track", "id", track.ID())
	}
}

func (b *Binding) onDataChannelMessage(msg pionwebrtc.DataChannelMessage) {
	if b.closed.Load() {
		return
	}
	if len(msg.Data) > maxDataChannelPacketSize {
		slog.WarnContext(b.ctx, "webrtc datachannel packet too large", "size", len(msg.Data))
		return
	}
	packet := protocol.AcquirePacket()
	if err := packet.Unmarshal(msg.Data); err != nil {
		protocol.ReleasePacket(packet)
		slog.WarnContext(b.ctx, "webrtc datachannel unmarshal packet failed", "error", err)
		return
	}

	b.deliver(packet)
}

// deliver hands one complete frame to the session. ReceivePacket only borrows
// the packet for the call, so it is released here.
func (b *Binding) deliver(packet *protocol.Packet) {
	defer protocol.ReleasePacket(packet)
	if b.session == nil {
		return
	}
	if err := b.session.ReceivePacket(b.lease, packet); err != nil {
		if errors.Is(err, engine.ErrSessionCloseInput) {
			b.close(false)
			if b.removeSession != nil {
				b.removeSession()
			}
			return
		}
		slog.DebugContext(b.ctx, "webrtc inbound packet rejected", "error", err)
	}
}

func (b *Binding) onConnectionStateChange(state pionwebrtc.PeerConnectionState) {
	slog.InfoContext(b.ctx, "webrtc peer connection state change", "state", state.String())
	switch state {
	case pionwebrtc.PeerConnectionStateConnected:
		b.stopDisconnectTimer()
		b.connected.Store(true)
		go b.sendLoop()
		b.resumeWorkflow()
	case pionwebrtc.PeerConnectionStateDisconnected:
		b.offline()
		b.armDisconnectTimer()
	case pionwebrtc.PeerConnectionStateFailed, pionwebrtc.PeerConnectionStateClosed:
		b.close(true)
	}
}

func (b *Binding) armDisconnectTimer() {
	if b.closed.Load() {
		return
	}
	timer := time.AfterFunc(disconnectGrace, func() {
		if !b.connected.Load() && !b.closed.Load() {
			b.close(true)
		}
	})
	if old := b.disconnectTimer.Swap(timer); old != nil {
		old.Stop()
	}
}

func (b *Binding) stopDisconnectTimer() {
	if old := b.disconnectTimer.Swap(nil); old != nil {
		old.Stop()
	}
}

func (b *Binding) offline() {
	b.connected.Store(false)
	b.queue.Reset()
	b.pauseWorkflow()
}

func (b *Binding) pauseWorkflow() {
	if b.closed.Load() {
		return
	}
	if b.session != nil && b.session.Workflow != nil && b.session.Workflow.State() == engine.WorkflowStateRunning {
		if err := b.session.Workflow.Pause(); err != nil {
			slog.DebugContext(b.ctx, "webrtc pause workflow skipped", "error", err)
		}
	}
}

func (b *Binding) resumeWorkflow() {
	if b.closed.Load() {
		return
	}
	if b.session != nil && b.session.Workflow != nil && b.session.Workflow.State() == engine.WorkflowStatePaused {
		if err := b.session.Workflow.Resume(); err != nil {
			slog.DebugContext(b.ctx, "webrtc resume workflow skipped", "error", err)
		}
	}
}

// close releases the WebRTC connection, signaling handles, and the session lease.
// pause decides whether the workflow transitions to paused upon Unbind:
// a service shutdown or disconnect pauses a living session, while an explicit deletion does not.
func (b *Binding) close(pause bool) {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	b.stopDisconnectTimer()
	b.connected.Store(false)

	// Notify client via DataChannel if still connected.
	if b.dcReady.Load() {
		packet := protocol.AcquirePacket()
		packet.Type = protocol.TypeClose
		b.sendDataChannelMessage(packet)
		protocol.ReleasePacket(packet)
	}

	b.cancel()
	b.queue.Reset()

	if b.media != nil {
		b.media.Close()
	}

	if b.provider != nil && b.handle != "" {
		go func(p Provider, h string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.Teardown(ctx, h)
		}(b.provider, b.handle)
	}

	if b.session != nil {
		b.session.Unbind(b.lease, pause)
	}

	if b.onClosed != nil {
		b.onClosed()
	}
}
