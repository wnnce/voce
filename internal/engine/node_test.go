package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

type mockPlugin struct {
	BuiltinPlugin
	onAudio      func(audio schema.Audio)
	onSignal     func(signal schema.Signal)
	onStartCount atomic.Int32
	onReadyCount atomic.Int32
	onStopCount  atomic.Int32
}

func (m *mockPlugin) OnStart(ctx context.Context, flow Flow) error {
	m.onStartCount.Add(1)
	return nil
}

func (m *mockPlugin) OnReady(ctx context.Context, flow Flow) {
	m.onReadyCount.Add(1)
}

func (m *mockPlugin) OnStop() {
	m.onStopCount.Add(1)
}

func (m *mockPlugin) OnAudio(ctx context.Context, flow Flow, audio schema.Audio) {
	if m.onAudio != nil {
		m.onAudio(audio)
	}
}

func (m *mockPlugin) OnSignal(ctx context.Context, flow Flow, signal schema.Signal) {
	if m.onSignal != nil {
		m.onSignal(signal)
	}
}

func TestNode_Lifecycle(t *testing.T) {
	plg := &mockPlugin{}
	ctx, cancel := context.WithCancel(context.Background())

	n := newNode(ctx, "test-node", plg)

	// Test Start
	err := n.start()
	require.NoError(t, err)
	assert.True(t, n.running.Load())
	assert.Equal(t, int32(1), plg.onStartCount.Load())

	// Test Ready
	n.ready()
	assert.Equal(t, int32(1), plg.onReadyCount.Load())

	// Test Stop
	n.stop()
	assert.False(t, n.running.Load())
	cancel()
	assert.Eventually(t, func() bool {
		return plg.onStopCount.Load() == 1
	}, 100*time.Millisecond, 10*time.Millisecond, "OnStop should be called after readLoop exits")
}

func TestNode_DataFlow(t *testing.T) {
	var wg sync.WaitGroup
	plg := &mockPlugin{
		onAudio: func(audio schema.Audio) {
			assert.Equal(t, "test-audio", audio.Name())
			wg.Done()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n := newNode(ctx, "test-node", plg)
	err := n.start()
	require.NoError(t, err)

	wg.Add(1)
	audio := schema.NewAudio("test-audio", AudioSampleRate, AudioChannels).ReadOnly()
	n.Input(audio)

	// Wait for processing
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for audio processing")
	}
}

func TestNode_RefCounting(t *testing.T) {
	var releaseCount atomic.Int32
	var retainCount atomic.Int32
	var wg sync.WaitGroup

	plg := &mockPlugin{
		onAudio: func(audio schema.Audio) {
			// Simulating processing
			wg.Done()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n1 := newNode(ctx, "sender", plg)
	n2 := newNode(ctx, "receiver", plg)

	_ = n1.start()
	_ = n2.start()

	n1.addNextNode(EventAudio, n2)

	audio := &mockTrackingAudio{
		onRetain:  func() { retainCount.Add(1) },
		onRelease: func() { releaseCount.Add(1) },
	}

	wg.Add(1)

	n1.SendAudio(audio)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		time.Sleep(10 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, int32(1), retainCount.Load(), "Should have retained once for downstream")
		assert.Equal(t, int32(1), releaseCount.Load(), "Should have released once after processing")
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for ref count lifecycle")
	}
}

type mockTrackingAudio struct {
	schema.Audio
	onRetain  func()
	onRelease func()
}

func (m *mockTrackingAudio) Retain() {
	if m.onRetain != nil {
		m.onRetain()
	}
}

func (m *mockTrackingAudio) Release() {
	if m.onRelease != nil {
		m.onRelease()
	}
}

func (m *mockTrackingAudio) Name() string { return "track" }

func TestNode_Backpressure(t *testing.T) {
	plg := &mockPlugin{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a small buffer if possible or just fill it.
	// The constants are fixed, so we fill 128 (audioBufferSize)
	n := newNode(ctx, "test-node", plg)
	// Don't call start() so readLoop doesn't consume
	n.running.Store(true)

	for i := 0; i < audioBufferSize; i++ {
		audio := schema.NewAudio("fill", AudioSampleRate, AudioChannels)
		n.Input(audio.ReadOnly())
	}

	// Next one should be dropped
	audio := schema.NewAudio("drop", AudioSampleRate, AudioChannels)
	n.Input(audio.ReadOnly())
	// No log check easily here without custom logger, but we can check if it finishes.
}

func TestNode_Priority(t *testing.T) {
	var processed []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	plg := &mockPlugin{
		onSignal: func(signal schema.Signal) {
			mu.Lock()
			processed = append(processed, "signal")
			mu.Unlock()
			wg.Done()
		},
		onAudio: func(audio schema.Audio) {
			mu.Lock()
			processed = append(processed, "audio")
			mu.Unlock()
			wg.Done()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n := newNode(ctx, "test-node", plg)
	// Don't start yet, fill both channels
	n.running.Store(true)

	for i := 0; i < 5; i++ {
		n.audioChan <- schema.NewAudio("a", AudioSampleRate, AudioChannels).ReadOnly()
	}
	n.signalChan <- schema.NewSignal("c").ReadOnly()

	wg.Add(6)
	go n.readLoop()

	wg.Wait()

	mu.Lock()
	// Signal should be first because of priority select
	assert.Equal(t, "signal", processed[0])
	mu.Unlock()
}

func BenchmarkNode_Input_Audio(b *testing.B) {
	plg := &BuiltinPlugin{}
	ctx := context.Background()
	n := newNode(ctx, "bench", plg)
	n.running.Store(true)
	go n.readLoop()

	audio := schema.NewAudio("bench", AudioSampleRate, AudioChannels).ReadOnly()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		audio.Retain()
		n.Input(audio)
	}
}

func BenchmarkNode_Dispatch_Audio(b *testing.B) {
	plg := &BuiltinPlugin{}
	ctx := context.Background()
	n1 := newNode(ctx, "n1", plg)
	n2 := newNode(ctx, "n2", plg)

	n1.running.Store(true)
	n2.running.Store(true)
	n1.addNextNode(EventAudio, n2)

	go n1.readLoop()
	go n2.readLoop()

	audio := schema.NewAudio("bench", AudioSampleRate, AudioChannels).ReadOnly()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		audio.Retain()
		n1.SendAudio(audio)
	}
}

func TestNode_Publish(t *testing.T) {
	plg := &mockPlugin{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n := newNode(ctx, "test-node", plg)

	packetChan := make(chan *protocol.Packet, 1)
	writer := &mockSocketWriter{
		onWrite: func(msg *protocol.Packet) {
			packetChan <- msg
		},
	}
	n.setSocketWriter(writer)

	payload := []byte("hello")
	n.Publish(protocol.TypeText, payload)

	select {
	case packet := <-packetChan:
		assert.Equal(t, protocol.TypeText, packet.Type)
		assert.Equal(t, payload, packet.Payload)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for packet publish")
	}
}

type mockSocketWriter struct {
	onWrite func(msg *protocol.Packet)
}

func (m *mockSocketWriter) Write(msg *protocol.Packet) {
	if m.onWrite != nil {
		m.onWrite(msg)
	}
}
