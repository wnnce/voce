package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"

	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/syncx"
)

// internalData holds the event data paired with a epoch for stale data detection.
type internalData[T schema.ReadOnly] struct {
	epoch int32
	data  T
}

// track acts as an independent execution pipeline for a specific event type (Audio, Video, or Payload).
// Each track runs in its own goroutine, allowing parallel processing of different media streams.
type track[T schema.ReadOnly] struct {
	ch       chan internalData[T]
	strategy DropStrategy
	epoch    atomic.Int32
	cancel   atomic.Pointer[context.CancelFunc] // Points to the cancel function of the currently executing event
	signals  []string                           // Signals that trigger a epoch bump and task cancellation for this track
}

func newStream[T schema.ReadOnly](bufSize int, strategy DropStrategy, signals []string) *track[T] {
	return &track[T]{
		ch:       make(chan internalData[T], bufSize),
		strategy: strategy,
		signals:  signals,
	}
}

func (s *track[T]) push(ctx context.Context, data T) {
	item := internalData[T]{
		epoch: s.epoch.Load(),
		data:  data,
	}
	if s.strategy == DropNewest {
		if err := syncx.SendWithNonBlocking(ctx, s.ch, item); err != nil {
			if rc, ok := any(data).(schema.RefCountable); ok {
				rc.Release()
			}
			slog.WarnContext(ctx, "stream buffer full, drop newest frame",
				"type", fmt.Sprintf("%T", data))
		}
		return
	}
	if err := syncx.SendWithContext(ctx, s.ch, item); err != nil {
		if rc, ok := any(data).(schema.RefCountable); ok {
			rc.Release()
		}
		slog.ErrorContext(ctx, "stream input blocked and context canceled",
			"type", fmt.Sprintf("%T", data))
	}
}

func (s *track[T]) readLoop(
	ctx context.Context,
	flow Flow,
	paused *atomic.Bool,
	makeContext bool,
	handler func(context.Context, Flow, T),
) {
	defer func() {
		for {
			select {
			case itl := <-s.ch:
				if ref, ok := any(itl.data).(schema.RefCountable); ok {
					ref.Release()
				}
			default:
				return
			}
		}
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case itl := <-s.ch:
			if paused.Load() || itl.epoch != s.epoch.Load() {
				if rc, ok := any(itl.data).(schema.RefCountable); ok {
					rc.Release()
				}
				continue
			}
			var curCtx = ctx
			var cancel context.CancelFunc
			if makeContext {
				curCtx, cancel = context.WithCancel(ctx)
				s.cancel.Store(&cancel)
			}
			processEvent(curCtx, flow, itl.data, handler)
			if cancel != nil {
				cancel()
				s.cancel.Store(nil)
			}
		}
	}
}

type TrackOption func(*MultiTrackPlugin)

func WithPayloadTrack(bufSize int, strategy DropStrategy, signals ...string) TrackOption {
	return func(w *MultiTrackPlugin) {
		w.payloadTrack = newStream[schema.Payload](bufSize, strategy, signals)
	}
}

func WithAudioTrack(bufSize int, strategy DropStrategy, signals ...string) TrackOption {
	return func(w *MultiTrackPlugin) {
		w.audioTrack = newStream[schema.Audio](bufSize, strategy, signals)
	}
}

func WithVideoTrack(bufSize int, strategy DropStrategy, signals ...string) TrackOption {
	return func(w *MultiTrackPlugin) {
		w.videoTrack = newStream[schema.Video](bufSize, strategy, signals)
	}
}

// MultiTrackPlugin is a high-performance middleware that provides parallel event processing
// and configurable backpressure (drop strategies) for any Plugin.
//
// Concurrency Model:
// Unlike a standard Node which processes all event types sequentially in a single loop,
// MultiTrackPlugin spawns separate goroutines (tracks) for Audio, Video, and Payload.
//
// Warning: The wrapped Plugin MUST BE THREAD-SAFE if multiple tracks are configured,
// as OnAudio, OnVideo, and OnPayload may be called concurrently from different goroutines.
//
// Key Features:
//  1. Parallelism: Prevents a slow LLM/Payload handler from blocking real-time Audio streaming.
//  2. Interruption: Specific signals (e.g., "interrupt") can cancel the context of active tasks
//     and discard all queued items in the track's buffer.
//  3. Backpressure: Supports DropNewest or Block strategies to manage system load.
type MultiTrackPlugin struct {
	ctx    context.Context
	flow   Flow
	plugin Plugin
	paused atomic.Bool

	payloadTrack *track[schema.Payload]
	audioTrack   *track[schema.Audio]
	videoTrack   *track[schema.Video]
}

func NewMultiTrackPlugin(plugin Plugin, options ...TrackOption) Plugin {
	if plugin == nil || len(options) == 0 {
		return plugin
	}
	wrapper := &MultiTrackPlugin{
		plugin: plugin,
	}
	for _, opt := range options {
		opt(wrapper)
	}
	return wrapper
}

func (b *MultiTrackPlugin) OnStart(ctx context.Context, flow Flow) error {
	if err := b.plugin.OnStart(ctx, flow); err != nil {
		return err
	}
	b.ctx = ctx
	b.flow = flow
	if b.payloadTrack != nil {
		go b.payloadTrack.readLoop(ctx, flow, &b.paused, true, b.plugin.OnPayload)
	}
	if b.audioTrack != nil {
		go b.audioTrack.readLoop(ctx, flow, &b.paused, true, b.plugin.OnAudio)
	}
	if b.videoTrack != nil {
		go b.videoTrack.readLoop(ctx, flow, &b.paused, true, b.plugin.OnVideo)
	}
	return nil
}

func (b *MultiTrackPlugin) OnReady(ctx context.Context, flow Flow) {
	b.plugin.OnReady(ctx, flow)
}

func (b *MultiTrackPlugin) OnPause(ctx context.Context) {
	b.paused.Store(true)
	b.plugin.OnPause(ctx)
}

func (b *MultiTrackPlugin) OnResume(ctx context.Context, flow Flow) {
	b.paused.Store(false)
	b.plugin.OnResume(ctx, flow)
}

func (b *MultiTrackPlugin) OnStop() {
	b.plugin.OnStop()
}

func (b *MultiTrackPlugin) OnSignal(ctx context.Context, flow Flow, signal schema.Signal) {
	name := signal.Name()
	interruptTrack(b.payloadTrack, name)
	interruptTrack(b.audioTrack, name)
	interruptTrack(b.videoTrack, name)
	b.plugin.OnSignal(ctx, flow, signal)
}

func interruptTrack[T schema.ReadOnly](s *track[T], signalName string) {
	if s == nil || !slices.Contains(s.signals, signalName) {
		return
	}
	s.epoch.Add(1)
	if cancel := s.cancel.Swap(nil); cancel != nil {
		(*cancel)()
	}
}

func (b *MultiTrackPlugin) OnPayload(ctx context.Context, flow Flow, payload schema.Payload) {
	if b.payloadTrack == nil {
		b.plugin.OnPayload(ctx, flow, payload)
		return
	}
	b.payloadTrack.push(ctx, payload)
}

func (b *MultiTrackPlugin) OnAudio(ctx context.Context, flow Flow, audio schema.Audio) {
	if b.audioTrack == nil {
		b.plugin.OnAudio(ctx, flow, audio)
		return
	}
	audio.Retain()
	b.audioTrack.push(ctx, audio)
}

func (b *MultiTrackPlugin) OnVideo(ctx context.Context, flow Flow, video schema.Video) {
	if b.videoTrack == nil {
		b.plugin.OnVideo(ctx, flow, video)
		return
	}
	video.Retain()
	b.videoTrack.push(ctx, video)
}

func processEvent[T schema.ReadOnly](
	ctx context.Context,
	flow Flow,
	event T,
	handler func(context.Context, Flow, T),
) {
	func() {
		defer func() {
			if ref, ok := any(event).(schema.RefCountable); ok {
				ref.Release()
			}
			if err := recover(); err != nil {
				slog.ErrorContext(ctx, "plugin panic recovered", "error", err)
			}
		}()
		handler(ctx, flow, event)
	}()
}
