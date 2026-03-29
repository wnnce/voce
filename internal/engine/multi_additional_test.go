package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wnnce/voce/internal/schema"
)

// TestMultiTrackPlugin_DropNewest verifies that when the payload track buffer
// is full and strategy is DropNewest, new items are silently dropped while
// existing items are eventually processed.
func TestMultiTrackPlugin_DropNewest(t *testing.T) {
	const bufSize = 3

	started := make(chan struct{}, 1)
	unblock := make(chan struct{})

	mock := &MockSlowPlugin{}
	var processed atomic.Int32

	mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
		select {
		case started <- struct{}{}: // signal first execution started
		default:
		}
		<-unblock
		processed.Add(1)
	}

	wrapped := NewMultiTrackPlugin(mock, WithPayloadTrack(bufSize, DropNewest))
	tester := NewPluginTester(t, wrapped).Start()
	defer tester.Stop()

	// Inject the first payload and wait for the goroutine to pick it up
	// (it will block in <-unblock, keeping the channel worker busy).
	p0 := schema.NewPayload("blocker")
	_ = p0.Set("id", 0)
	tester.InjectPayload(p0.ReadOnly())

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first payload never started processing")
	}

	// Now inject bufSize payloads to fill the buffer, plus 2 extras that
	// should be dropped because the buffer is already full.
	for i := 1; i <= bufSize+2; i++ {
		p := schema.NewPayload("test")
		_ = p.Set("id", i)
		tester.InjectPayload(p.ReadOnly())
	}

	// Unblock all workers.
	close(unblock)
	time.Sleep(300 * time.Millisecond)

	// The goroutine was blocking on 1 item; bufSize more fit in the channel;
	// the remaining 2 should have been dropped.
	// So total processed ≤ 1 (blocker) + bufSize.
	assert.LessOrEqual(t, processed.Load(), int32(1+bufSize),
		"items beyond buffer capacity should be dropped with DropNewest")
	assert.Positive(t, processed.Load(),
		"at least the first (blocker) item should be processed")
}

// TestMultiTrackPlugin_PauseResume verifies that payloads received while
// the MultiTrackPlugin is paused are discarded, and that normal processing
// resumes after OnResume.
func TestMultiTrackPlugin_PauseResume(t *testing.T) {
	mock := &MockSlowPlugin{}
	var processed atomic.Int32

	mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
		processed.Add(1)
	}

	wrapped := NewMultiTrackPlugin(mock, WithPayloadTrack(16, BlockIfFull))
	tester := NewPluginTester(t, wrapped).Start()
	defer tester.Stop()

	// Pause through the wrapper.
	wrapped.OnPause(tester.Context())
	time.Sleep(20 * time.Millisecond)

	// Inject several payloads while paused — all should be dropped by the track.
	for range 3 {
		tester.InjectPayload(schema.NewPayload("during-pause").ReadOnly())
	}
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), processed.Load(), "no payloads should be processed while paused")

	// Resume and send one more payload — must be processed.
	wrapped.OnResume(tester.Context(), tester.mock)
	tester.InjectPayload(schema.NewPayload("after-resume").ReadOnly())
	assert.Eventually(t, func() bool { return processed.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond,
		"payload after resume must be processed")
}

// TestMultiTrackPlugin_Passthrough verifies that without any track options,
// OnPayload is called synchronously (no buffering goroutine).
func TestMultiTrackPlugin_Passthrough(t *testing.T) {
	mock := &MockSlowPlugin{}
	var processed atomic.Int32

	mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
		processed.Add(1)
	}

	// No options → NewMultiTrackPlugin just returns the original plugin.
	result := NewMultiTrackPlugin(mock)
	assert.Equal(t, mock, result, "without options the original plugin should be returned")

	// Calling OnPayload directly should still work.
	ctx := context.Background()
	result.OnPayload(ctx, &MockFlow{}, schema.NewPayload("test").ReadOnly())
	assert.Equal(t, int32(1), processed.Load(), "passthrough plugin must dispatch OnPayload directly")
}

// TestMultiTrackPlugin_VideoTrack verifies that a video track is dispatched
// concurrently with payload, so a slow payload handler does not block video.
func TestMultiTrackPlugin_VideoTrack(t *testing.T) {
	mock := &MockSlowPlugin{}

	payloadStarted := make(chan struct{}, 1)
	videoProcessed := make(chan struct{}, 1)

	mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
		payloadStarted <- struct{}{}
		time.Sleep(200 * time.Millisecond) // slow payload
	}

	// We need a video hook — MockSlowPlugin doesn't have one, create a wrapper.
	videoMock := &videoCaptureMock{
		inner: mock,
		onVideo: func() {
			videoProcessed <- struct{}{}
		},
	}

	wrapped := NewMultiTrackPlugin(videoMock,
		WithPayloadTrack(4, BlockIfFull),
		WithVideoTrack(4, DropNewest),
	)

	tester := NewPluginTester(t, wrapped).Start()
	defer tester.Stop()

	// Kick off a slow payload.
	tester.InjectPayload(schema.NewPayload("slow").ReadOnly())
	select {
	case <-payloadStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("payload never started")
	}

	// Inject a video frame — must be processed WITHOUT waiting for the slow payload.
	video := schema.NewVideo("test-video", 720, 1280, 33*time.Millisecond).ReadOnly()
	tester.InjectVideo(video)

	select {
	case <-videoProcessed:
		// OK — video was processed while payload was still running.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("video was blocked by slow payload handler")
	}
}

// TestMultiTrackPlugin_MultipleSignalsPerTrack verifies that a track
// configured with multiple interrupt signals correctly responds to any of them.
func TestMultiTrackPlugin_MultipleSignalsPerTrack(t *testing.T) {
	mock := &MockSlowPlugin{}
	var canceled atomic.Bool

	mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
		select {
		case <-ctx.Done():
			canceled.Store(true)
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Track listens for two different interrupt signal names.
	wrapped := NewMultiTrackPlugin(mock,
		WithPayloadTrack(8, BlockIfFull, "interrupt-a", "interrupt-b"),
	)

	tester := NewPluginTester(t, wrapped).Start()
	defer tester.Stop()

	tester.InjectPayload(schema.NewPayload("slow").ReadOnly())
	time.Sleep(20 * time.Millisecond)

	// Fire the second signal name — should still cancel the active context.
	tester.InjectSignal(schema.NewSignal("interrupt-b").ReadOnly())
	assert.Eventually(t, func() bool { return canceled.Load() },
		300*time.Millisecond, 5*time.Millisecond,
		"interrupt-b should cancel the active payload context")
}

// videoCaptureMock wraps a plugin and adds an OnVideo hook.
type videoCaptureMock struct {
	BuiltinPlugin
	inner   Plugin
	onVideo func()
}

func (v *videoCaptureMock) OnStart(ctx context.Context, flow Flow) error {
	return v.inner.OnStart(ctx, flow)
}

func (v *videoCaptureMock) OnPayload(ctx context.Context, flow Flow, p schema.Payload) {
	v.inner.OnPayload(ctx, flow, p)
}

func (v *videoCaptureMock) OnSignal(ctx context.Context, flow Flow, s schema.Signal) {
	v.inner.OnSignal(ctx, flow, s)
}

func (v *videoCaptureMock) OnVideo(_ context.Context, _ Flow, _ schema.Video) {
	if v.onVideo != nil {
		v.onVideo()
	}
}
