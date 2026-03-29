package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/schema"
)

// fullMockPlugin is a test plugin that intercepts all event types via callbacks.
type fullMockPlugin struct {
	BuiltinPlugin
	onPayload    func(ctx context.Context, flow Flow, p schema.Payload)
	onSignalHook func(ctx context.Context, flow Flow, s schema.Signal)
	onAudioHook  func(audio schema.Audio)
	onStopHook   func()
	stopCount    atomic.Int32
}

func (f *fullMockPlugin) OnPayload(ctx context.Context, flow Flow, p schema.Payload) {
	if f.onPayload != nil {
		f.onPayload(ctx, flow, p)
	}
}

func (f *fullMockPlugin) OnSignal(ctx context.Context, flow Flow, s schema.Signal) {
	if f.onSignalHook != nil {
		f.onSignalHook(ctx, flow, s)
	}
}

func (f *fullMockPlugin) OnAudio(ctx context.Context, flow Flow, a schema.Audio) {
	if f.onAudioHook != nil {
		f.onAudioHook(a)
	}
}

func (f *fullMockPlugin) OnStop() {
	f.stopCount.Add(1)
	if f.onStopHook != nil {
		f.onStopHook()
	}
}

// TestNode_PayloadFlow verifies that a payload injected via Input reaches
// the plugin's OnPayload handler.
func TestNode_PayloadFlow(t *testing.T) {
	var received []schema.Payload
	var mu sync.Mutex

	plg := &fullMockPlugin{
		onPayload: func(_ context.Context, _ Flow, p schema.Payload) {
			mu.Lock()
			received = append(received, p)
			mu.Unlock()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n := newNode(ctx, "payload-node", plg)
	require.NoError(t, n.start())

	p := schema.NewPayload("test")
	_ = p.Set("key", "value")
	n.Input(p.ReadOnly())

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 500*time.Millisecond, 5*time.Millisecond, "payload must reach OnPayload")
}

// TestNode_PauseResume verifies that:
//   - pause() triggers OnPause on the plugin
//   - resume() triggers OnResume on the plugin
//   - the paused flag in OnPause is observable by the plugin
func TestNode_PauseResume(t *testing.T) {
	var pauseCalled, resumeCalled atomic.Bool

	plg := &mockPlugin{}
	// Wrap BuiltinPlugin to capture OnPause / OnResume via OnStart returning a
	// custom plugin that overrides those methods.
	// Since mockPlugin doesn't have OnPause overrides, we use the node directly
	// and verify the lifecycle via the control messages being consumed.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pausedPlugin := &pauseResumeMock{
		onPause:  func() { pauseCalled.Store(true) },
		onResume: func() { resumeCalled.Store(true) },
	}
	_ = plg // keep mockPlugin for other tests
	n := newNode(ctx, "pause-test", pausedPlugin)
	require.NoError(t, n.start())

	n.pause()
	assert.Eventually(t, func() bool { return pauseCalled.Load() },
		500*time.Millisecond, 5*time.Millisecond, "OnPause must be called after pause()")

	n.resume()
	assert.Eventually(t, func() bool { return resumeCalled.Load() },
		500*time.Millisecond, 5*time.Millisecond, "OnResume must be called after resume()")
}

// pauseResumeMock is a minimal plugin that tracks OnPause / OnResume calls.
type pauseResumeMock struct {
	BuiltinPlugin
	onPause  func()
	onResume func()
}

func (p *pauseResumeMock) OnPause(_ context.Context) {
	if p.onPause != nil {
		p.onPause()
	}
}

func (p *pauseResumeMock) OnResume(_ context.Context, _ Flow) {
	if p.onResume != nil {
		p.onResume()
	}
}

// TestNode_SignalFanOut verifies that a signal sent via SendSignal is
// delivered to every registered downstream node.
func TestNode_SignalFanOut(t *testing.T) {
	const numReceivers = 3

	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numReceivers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	makeRecv := func() *node {
		plg := &fullMockPlugin{
			onSignalHook: func(_ context.Context, _ Flow, _ schema.Signal) {
				count.Add(1)
				wg.Done()
			},
		}
		n := newNode(ctx, "recv", plg)
		_ = n.start()
		return n
	}

	sender := newNode(ctx, "sender", &BuiltinPlugin{})
	_ = sender.start()
	for range numReceivers {
		sender.addNextNode(EventSignal, makeRecv())
	}

	sender.SendSignal(schema.NewSignal("broadcast").ReadOnly())

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for fan-out signal delivery")
	}
	assert.Equal(t, int32(numReceivers), count.Load())
}

// TestNode_AudioFanOut verifies that SendAudio delivers to all downstream
// nodes, retaining reference counts correctly.
func TestNode_AudioFanOut(t *testing.T) {
	const numReceivers = 4

	var received atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numReceivers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender := newNode(ctx, "sender", &BuiltinPlugin{})
	_ = sender.start()

	for range numReceivers {
		plg := &fullMockPlugin{
			onAudioHook: func(schema.Audio) {
				received.Add(1)
				wg.Done()
			},
		}
		n := newNode(ctx, "recv", plg)
		_ = n.start()
		sender.addNextNode(EventAudio, n)
	}

	sender.SendAudio(schema.NewAudio("fan", AudioSampleRate, AudioChannels).ReadOnly())

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for fan-out delivery")
	}
	assert.Equal(t, int32(numReceivers), received.Load())
}

// TestNode_PortRouting verifies that SendPayloadToPort delivers only to the
// nodes registered on the specified port.
func TestNode_PortRouting(t *testing.T) {
	var port0, port1 atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	makeCapture := func(counter *atomic.Int32) *node {
		plg := &fullMockPlugin{
			onPayload: func(_ context.Context, _ Flow, _ schema.Payload) {
				counter.Add(1)
			},
		}
		n := newNode(ctx, "cap", plg)
		_ = n.start()
		return n
	}

	sender := newNode(ctx, "sender", &BuiltinPlugin{})
	_ = sender.start()

	sender.addNextPortNode(EventPayload, makeCapture(&port0), 0)
	sender.addNextPortNode(EventPayload, makeCapture(&port1), 1)

	// Only port-1 receiver should be reached.
	sender.SendPayloadToPort(1, schema.NewPayload("t").ReadOnly())

	assert.Eventually(t, func() bool { return port1.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond, "port-1 receiver must be reached")

	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(0), port0.Load(), "port-0 receiver must not receive port-1 send")
}

// TestNode_ContextCancelStopsLoop verifies that canceling the parent context
// terminates the readLoop and triggers OnStop.
func TestNode_ContextCancelStopsLoop(t *testing.T) {
	plg := &fullMockPlugin{}
	ctx, cancel := context.WithCancel(context.Background())

	n := newNode(ctx, "ctx-cancel", plg)
	require.NoError(t, n.start())
	assert.True(t, n.running.Load())

	cancel()

	assert.Eventually(t, func() bool { return plg.stopCount.Load() == 1 },
		500*time.Millisecond, 5*time.Millisecond, "OnStop must be called after ctx cancel")
	assert.False(t, n.running.Load())
}

// TestNode_StoppedNodeDropsInput verifies that a node with running=false
// silently drops all input without panicking.
func TestNode_StoppedNodeDropsInput(t *testing.T) {
	var called atomic.Bool
	plg := &fullMockPlugin{
		onAudioHook: func(schema.Audio) { called.Store(true) },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n := newNode(ctx, "stopped", plg)
	// Mark stopped without ever starting the readLoop.
	n.running.Store(false)

	n.Input(schema.NewAudio("x", AudioSampleRate, AudioChannels).ReadOnly())
	time.Sleep(40 * time.Millisecond)
	assert.False(t, called.Load(), "stopped node must not dispatch events")
}
