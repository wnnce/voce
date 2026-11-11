package webrtc

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/pion/rtp/codecs"
	pionwebrtc "github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media/samplebuilder"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
	"github.com/wnnce/voce/pkg/buf"
	"github.com/wnnce/voce/pkg/pool"
)

const (
	// audioSampleQueueCapacity caps pending Opus samples at 25 frames (~500ms).
	audioSampleQueueCapacity = 25
	defaultSampleCapacity    = 1500
)

var audioSamplePool = pool.NewTypedPool(func() *buf.Buffer {
	return &buf.Buffer{
		Buf:        make([]byte, 0, defaultSampleCapacity),
		RecycleCap: defaultSampleCapacity,
	}
})

// startAudioTrackWorker initializes the decoupled audio pipeline and launches readLoop and decodeLoop.
func (b *Binding) startAudioTrackWorker(track *pionwebrtc.TrackRemote) {
	workerCtx, cancel := context.WithCancel(b.ctx)
	samples := make(chan *buf.Buffer, audioSampleQueueCapacity)

	go b.readLoop(workerCtx, track, samples)
	go b.decodeLoop(workerCtx, samples, cancel)
}

// readLoop continuously reads RTP packets from the network, feeds the jitter buffer,
// and pushes reassembled Opus samples into the bounded queue.
func (b *Binding) readLoop(ctx context.Context, track *pionwebrtc.TrackRemote, samples chan<- *buf.Buffer) {
	defer close(samples)

	depacketizer := &codecs.OpusPacket{}
	sb := pionmedia.New(12, depacketizer, 48000)

	for {
		if ctx.Err() != nil || b.closed.Load() {
			return
		}

		packet, _, err := track.ReadRTP()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			if ctx.Err() != nil || b.closed.Load() {
				return
			}
			slog.WarnContext(ctx, "webrtc read rtp packet failed", "id", track.ID(), "error", err)
			return
		}

		sb.Push(packet)
		for sample := sb.Pop(); sample != nil; sample = sb.Pop() {
			if len(sample.Data) == 0 {
				continue
			}

			sampleBuf := audioSamplePool.Acquire()
			sampleBuf.Set(sample.Data)
			select {
			case <-ctx.Done():
				audioSamplePool.Release(sampleBuf)
				return
			case samples <- sampleBuf:
			default:
				// Channel is full: drop the newest sample explicitly to prevent socket buffer backlog.
				audioSamplePool.Release(sampleBuf)
				slog.WarnContext(ctx, "webrtc inbound audio queue full, dropping sample", "track", track.ID(), "size", len(sample.Data))
			}
		}
	}
}

// decodeLoop consumes Opus samples from the channel, decodes them to PCM16 via CGO libopus,
// aggregates 20ms chunks into 100ms frames, and delivers to the session.
func (b *Binding) decodeLoop(ctx context.Context, samples <-chan *buf.Buffer, cancel context.CancelFunc) {
	if cancel != nil {
		defer cancel()
	}

	decoder, err := media.NewAudioDecoder()
	if err != nil {
		slog.WarnContext(ctx, "webrtc init audio decoder failed", "error", err)
		for s := range samples {
			audioSamplePool.Release(s)
		}
		return
	}

	defer func() {
		// Ensure any remaining buffers in samples are returned to pool upon exit.
		for s := range samples {
			audioSamplePool.Release(s)
		}
	}()

	inBuffer := make([]byte, engine.AudioBufferSize)
	inUsed := 0

	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-samples:
			if !ok {
				// Producer finished and queue drained cleanly.
				return
			}

			pcm, decodeErr := decoder.Decode(s.Buf)
			audioSamplePool.Release(s)
			if decodeErr != nil {
				slog.WarnContext(ctx, "webrtc decode remote opus failed", "error", decodeErr)
				continue
			}
			if len(pcm) == 0 || b.closed.Load() || !b.connected.Load() {
				inUsed = 0
				continue
			}

			for offset := 0; offset < len(pcm); {
				written := copy(inBuffer[inUsed:], pcm[offset:])
				inUsed += written
				offset += written
				if inUsed < len(inBuffer) {
					continue
				}

				packet := protocol.AcquirePacket()
				packet.Type = protocol.TypeAudio
				packet.SetPayload(inBuffer)
				b.deliver(packet)
				inUsed = 0
			}
		}
	}
}
