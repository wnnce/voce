package agora

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	agoraservice "github.com/AgoraIO-Extensions/Agora-Golang-Server-SDK/v2/go_sdk/rtc"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/buf"
)

const (
	// audioFrameInterval is the outbound pacing tick: one 10ms frame per tick.
	audioFrameInterval = 10 * time.Millisecond
	// audioBlockSize is one second of 16k mono PCM16, the shared pool block size.
	audioBlockSize = engine.AudioFrameSize * 100
	// audioQueueLimit caps the pending audio at one second (32KB, far below any
	// memory concern); audio arriving when the queue is full is dropped.
	audioQueueLimit = audioBlockSize
)

// Binding adapts one Agora connection to one Session. It receives the client's
// PCM, aggregates it into Session-sized frames, and owns the connection together
// with the session lease that authorizes it.
//
// Callbacks are installed by New and may run concurrently; the SDK owns the
// frame buffers for the duration of a call only.
type Binding struct {
	session *engine.Session
	lease   engine.Lease
	cred    Credentials
	conn    *agoraservice.RtcConnection
	ctx     context.Context
	cancel  context.CancelFunc

	connected atomic.Bool
	peer      atomic.Bool
	closed    atomic.Bool

	mu     sync.Mutex
	buffer []byte // inbound aggregation target
	used   int

	queue   *buf.ChunkQueue
	frame   []byte
	running atomic.Bool // the sender goroutine runs at most once at a time

	messenger *messenger
	rtmReady  atomic.Bool
}

// New wraps conn into a Binding and registers the connection callbacks. The
// connection is expected to be joined already.
func New(
	session *engine.Session,
	lease engine.Lease,
	cred Credentials,
	conn *agoraservice.RtcConnection,
	pool *buf.ChunkPool,
	msg *messenger,
) *Binding {
	ctx, cancel := context.WithCancel(session.Workflow.Context())
	binding := &Binding{
		session: session,
		lease:   lease,
		cred:    cred,
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
		buffer:  make([]byte, engine.AudioBufferSize),
		queue:   buf.NewChunkQueue(pool, audioQueueLimit),
		frame:   make([]byte, engine.AudioFrameSize),

		messenger: msg,
	}
	conn.RegisterObserver(&agoraservice.RtcConnectionObserver{
		OnConnected:         binding.OnConnected,
		OnDisconnected:      binding.OnDisconnected,
		OnConnecting:        binding.OnConnecting,
		OnReconnecting:      binding.OnReconnecting,
		OnReconnected:       binding.OnReconnected,
		OnConnectionLost:    binding.OnConnectionLost,
		OnConnectionFailure: binding.OnConnectionFailure,
		OnUserJoined:        binding.OnUserJoined,
		OnUserLeft:          binding.OnUserLeft,
		OnError:             binding.OnError,
	})
	if msg != nil {
		msg.attach(binding)
	}
	return binding
}

// Write is the session's PacketWriter: it copies outbound audio into the queue
// and drops pending audio when an interrupt arrives. It never blocks.
func (b *Binding) Write(packet *protocol.Packet) error {
	switch packet.Type {
	case protocol.TypeAudio:
		b.enqueue(packet.Payload)
	case protocol.TypePause, protocol.TypeResume:
		// Server-to-client control packets are driven by the transport, not
		// forwarded from the session.
	case protocol.TypeInterrupter:
		b.queue.Reset()
		b.sendMessage(packet)
	default:
		b.sendMessage(packet)
	}
	return nil
}

// sendMessage forwards one non-audio packet to the session's RTM topic. A
// packet that arrives before RTM finished joining is dropped.
func (b *Binding) sendMessage(packet *protocol.Packet) {
	if b.messenger == nil {
		return
	}
	b.messenger.publish(b, packet)
}

// enqueue copies one payload into the bounded queue. A full queue means the
// consumer fell behind, so the audio is dropped: blocking here would stall the
// session's output loop and discarding what is already queued would not help.
func (b *Binding) enqueue(pcm []byte) {
	if _, err := b.queue.Write(pcm); err != nil {
		slog.WarnContext(b.ctx, "rtc outbound audio dropped", "size", len(pcm), "error", err)
	}
}

// startSender launches the pacing loop. It may be called on every connect and
// every user join; the loop itself keeps a single instance running.
func (b *Binding) startSender() {
	if b.closed.Load() {
		return
	}
	go b.sendLoop()
}

// sendLoop paces queued audio to the SDK, one 10ms frame per tick. A late tick
// is not compensated: sending the backlog in a burst would sound worse than
// letting the queue drain.
func (b *Binding) sendLoop() {
	// RTC reconnects, so callbacks may start more than one loop over time.
	if !b.running.CompareAndSwap(false, true) {
		return
	}
	defer b.running.Store(false)
	ticker := time.NewTicker(audioFrameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
		}
		// select picks randomly when both cases are ready, and close may land
		// between two ticks, so the gate is rechecked before the connection is
		// touched.
		if b.closed.Load() || b.ctx.Err() != nil {
			return
		}
		if !b.connected.Load() || !b.peer.Load() {
			continue
		}
		read, err := b.queue.Read(b.frame)
		if read == 0 {
			if err != nil && !errors.Is(err, io.EOF) {
				slog.WarnContext(b.ctx, "rtc outbound audio read failed", "error", err)
			}
			continue
		}
		if read < len(b.frame) {
			// A partial tail is padded with silence so the SDK always gets a
			// whole 10ms frame.
			clear(b.frame[read:])
		}
		if code := b.conn.PushAudioPcmData(b.frame, engine.AudioSampleRate, engine.AudioChannels, 0); code != 0 {
			slog.WarnContext(b.ctx, "rtc outbound audio send failed", "code", code)
		}
	}
}

// OnConnected enables the audio path: the SDK only exposes the local user after
// the connection is up, so the frame observer is registered here rather than in
// New.
func (b *Binding) OnConnected(conn *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo, reason int) {
	slog.InfoContext(b.ctx, "rtc connected", "reason", reason, "channel", b.cred.Channel)
	b.connected.Store(true)
	conn.GetLocalUser().SetPlaybackAudioFrameBeforeMixingParameters(engine.AudioChannels, engine.AudioSampleRate)
	conn.RegisterAudioFrameObserver(&agoraservice.AudioFrameObserver{
		OnPlaybackAudioFrameBeforeMixing: b.OnPlaybackAudioFrameBeforeMixing,
	}, 0, nil)
	conn.PublishAudio()
	b.startSender()
	b.resumeWorkflow()
}

func (b *Binding) OnDisconnected(_ *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo, reason int) {
	slog.InfoContext(b.ctx, "rtc disconnected", "reason", reason)
	b.offline()
}

func (b *Binding) OnConnecting(_ *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo, reason int) {
	slog.InfoContext(b.ctx, "rtc connecting", "reason", reason)
}

// OnReconnecting fires on the way from CONNECTED to RECONNECTING without an
// intervening disconnect, so the media gate and the workflow are handled here.
func (b *Binding) OnReconnecting(_ *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo, reason int) {
	slog.InfoContext(b.ctx, "rtc reconnecting", "reason", reason)
	b.offline()
}

func (b *Binding) OnConnectionLost(_ *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo) {
	slog.WarnContext(b.ctx, "rtc connection lost, sdk keeps rejoining")
	b.offline()
}

func (b *Binding) OnReconnected(_ *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo, reason int) {
	slog.InfoContext(b.ctx, "rtc reconnected", "reason", reason)
	b.connected.Store(true)
	b.startSender()
	b.resumeWorkflow()
}

// OnConnectionFailure is terminal for the SDK: it stops rejoining. The workflow
// stays paused and the binding is left to the session idle cleanup.
func (b *Binding) OnConnectionFailure(_ *agoraservice.RtcConnection, _ *agoraservice.RtcConnectionInfo, errCode int) {
	slog.WarnContext(b.ctx, "rtc connection failure, sdk stops rejoining", "code", errCode)
	b.offline()
}

func (b *Binding) OnError(_ *agoraservice.RtcConnection, code int, message string) {
	slog.WarnContext(b.ctx, "rtc error", "code", code, "message", message)
}

// OnUserJoined marks the session's own client as present; other users in the
// channel belong to someone else and are ignored.
func (b *Binding) OnUserJoined(_ *agoraservice.RtcConnection, uid string) {
	if uid != b.cred.UID {
		slog.WarnContext(b.ctx, "ignoring unexpected remote user", "uid", uid)
		return
	}
	slog.InfoContext(b.ctx, "rtc client joined", "uid", uid)
	b.peer.Store(true)
	b.startSender()
	b.resumeWorkflow()
}

func (b *Binding) OnUserLeft(_ *agoraservice.RtcConnection, uid string, reason int) {
	if uid != b.cred.UID {
		return
	}
	slog.InfoContext(b.ctx, "rtc client left", "uid", uid, "reason", reason)
	b.offline()
}

// OnPlaybackAudioFrameBeforeMixing receives the client's PCM. The SDK delivers
// arbitrary chunk sizes, so the frame is buffered until one Session-sized block
// is complete; a partial block is dropped on disconnect.
func (b *Binding) OnPlaybackAudioFrameBeforeMixing(
	_ *agoraservice.LocalUser,
	_ string,
	uid string,
	frame *agoraservice.AudioFrame,
	_ agoraservice.VadState,
	_ *agoraservice.AudioFrame,
) bool {
	if uid != b.cred.UID || !b.connected.Load() || !b.peer.Load() || b.closed.Load() || frame == nil {
		return false
	}

	// The SDK is configured for 10ms frames, so one callback completes at most
	// one 100ms block; the packet is filled here and delivered outside the lock.
	b.mu.Lock()
	var ready *protocol.Packet
	for offset := 0; offset < len(frame.Buffer); {
		written := copy(b.buffer[b.used:], frame.Buffer[offset:])
		b.used += written
		offset += written
		if b.used < len(b.buffer) {
			continue
		}
		if ready == nil {
			ready = protocol.AcquirePacket()
			ready.Type = protocol.TypeAudio
			ready.SetPayload(b.buffer)
		}
		b.used = 0
	}
	b.mu.Unlock()

	if ready != nil {
		b.deliver(ready)
	}
	return true
}

// deliver hands one complete frame to the session. ReceivePacket only borrows
// the packet for the call, so it is released here.
func (b *Binding) deliver(packet *protocol.Packet) {
	defer protocol.ReleasePacket(packet)
	if err := b.session.ReceivePacket(b.lease, packet); err != nil {
		slog.DebugContext(b.ctx, "rtc inbound frame rejected", "error", err)
	}
}

// offline closes the media gate and pauses the workflow. The connection and the
// lease stay: the SDK reconnects on its own and the client may come back.
func (b *Binding) offline() {
	b.connected.Store(false)
	b.peer.Store(false)
	b.reset()
	b.pauseWorkflow()
}

// pauseWorkflow and resumeWorkflow tolerate repeated calls: the workflow only
// accepts each transition from one specific state.
func (b *Binding) pauseWorkflow() {
	if b.closed.Load() {
		return
	}
	if err := b.session.Workflow.Pause(); err != nil {
		slog.DebugContext(b.ctx, "rtc pause skipped", "error", err)
	}
}

func (b *Binding) resumeWorkflow() {
	if b.closed.Load() {
		return
	}
	if err := b.session.Workflow.Resume(); err != nil {
		slog.DebugContext(b.ctx, "rtc resume skipped", "error", err)
	}
}

func (b *Binding) reset() {
	b.mu.Lock()
	b.used = 0
	b.mu.Unlock()
}

// close releases the Agora connection and then the lease. pause decides whether
// the workflow goes back to paused, exactly like Unbind's own contract: a
// service shutdown pauses a still-living session, while a session deletion does
// not.
func (b *Binding) close(pause bool) {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	// Tell the client the session is over while the RTM channel is still up.
	if b.rtmReady.Load() && b.messenger != nil {
		packet := protocol.AcquirePacket()
		packet.Type = protocol.TypeClose
		b.messenger.publish(b, packet)
		protocol.ReleasePacket(packet)
	}

	b.cancel()
	b.queue.Reset()
	b.conn.UnpublishAudio()
	b.conn.Disconnect()
	b.conn.Release()
	if b.messenger != nil {
		b.messenger.detach(b)
	}
	b.session.Unbind(b.lease, pause)
}
