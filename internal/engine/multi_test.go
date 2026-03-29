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

type MockSlowPlugin struct {
	BuiltinPlugin
	OnPayloadFunc func(ctx context.Context, flow Flow, payload schema.Payload)
	OnAudioFunc   func(ctx context.Context, flow Flow, audio schema.Audio)
	OnSignalFunc  func(ctx context.Context, flow Flow, signal schema.Signal)
}

func (m *MockSlowPlugin) OnPayload(ctx context.Context, flow Flow, payload schema.Payload) {
	if m.OnPayloadFunc != nil {
		m.OnPayloadFunc(ctx, flow, payload)
	}
}

func (m *MockSlowPlugin) OnAudio(ctx context.Context, flow Flow, audio schema.Audio) {
	if m.OnAudioFunc != nil {
		m.OnAudioFunc(ctx, flow, audio)
	}
}

func (m *MockSlowPlugin) OnSignal(ctx context.Context, flow Flow, signal schema.Signal) {
	if m.OnSignalFunc != nil {
		m.OnSignalFunc(ctx, flow, signal)
	}
}

func TestMultiTrackPlugin(t *testing.T) {
	t.Run("Basic Serialization per Track", func(t *testing.T) {
		mock := &MockSlowPlugin{}
		var callOrder []string
		var mu sync.Mutex

		mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
			mu.Lock()
			callOrder = append(callOrder, "data:"+schema.GetAs[string](payload, "id", ""))
			mu.Unlock()
		}

		mock.OnAudioFunc = func(ctx context.Context, flow Flow, audio schema.Audio) {
			mu.Lock()
			callOrder = append(callOrder, "audio")
			mu.Unlock()
		}

		// Configure both payload and audio as buffered tracks
		wrapped := NewMultiTrackPlugin(mock,
			WithPayloadTrack(10, BlockIfFull, "interrupt"),
			WithAudioTrack(10, BlockIfFull, "interrupt"),
		)

		tester := NewPluginTester(t, wrapped).Start()
		defer tester.Stop()

		payload1 := schema.NewPayload("test")
		_ = payload1.Set("id", "1")
		payload2 := schema.NewPayload("test")
		_ = payload2.Set("id", "2")
		audio := schema.NewAudio(schema.AudioTTS, AudioSampleRate, AudioChannels).ReadOnly()

		tester.InjectPayload(payload1.ReadOnly())
		time.Sleep(10 * time.Millisecond)
		tester.InjectPayload(payload2.ReadOnly())
		time.Sleep(10 * time.Millisecond)
		tester.InjectAudio(audio)

		// Wait for all to be processed (approx 250ms)
		time.Sleep(250 * time.Millisecond)

		mu.Lock()
		// it should contains all injected items
		assert.Contains(t, callOrder, "data:1")
		assert.Contains(t, callOrder, "data:2")
		assert.Contains(t, callOrder, "audio")
		mu.Unlock()
	})

	t.Run("Interruption and Context Cancellation", func(t *testing.T) {
		mock := &MockSlowPlugin{}
		var payloadCanceled atomic.Bool
		var payloadProcessed atomic.Bool
		var signalReceived atomic.Bool

		interruptSignal := "interrupt"

		mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
			select {
			case <-ctx.Done():
				payloadCanceled.Store(true)
			case <-time.After(200 * time.Millisecond):
				payloadProcessed.Store(true)
			}
		}

		mock.OnSignalFunc = func(ctx context.Context, flow Flow, signal schema.Signal) {
			if signal.Name() == interruptSignal {
				signalReceived.Store(true)
			}
		}

		wrapped := NewMultiTrackPlugin(mock,
			WithPayloadTrack(10, BlockIfFull, interruptSignal),
		)

		tester := NewPluginTester(t, wrapped).Start()
		defer tester.Stop()

		tester.InjectPayload(schema.NewPayload("test").ReadOnly())
		time.Sleep(10 * time.Millisecond) // Ensure data starts processing

		tester.InjectSignal(schema.NewSignal(interruptSignal).ReadOnly())

		time.Sleep(100 * time.Millisecond)

		assert.True(t, signalReceived.Load(), "Signal should be received immediately")
		assert.True(t, payloadCanceled.Load(), "Payload should be canceled by signal")
		assert.False(t, payloadProcessed.Load(), "Payload should not complete processing")
	})

	t.Run("Stale Data Dropping", func(t *testing.T) {
		mock := &MockSlowPlugin{}
		var processedIDs []string
		var mu sync.Mutex

		clearSignal := "clear"

		mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
			id := schema.GetAs[string](payload, "id", "")
			mu.Lock()
			processedIDs = append(processedIDs, id)
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
		}

		wrapped := NewMultiTrackPlugin(mock,
			WithPayloadTrack(10, BlockIfFull, clearSignal),
		)

		tester := NewPluginTester(t, wrapped).Start()
		defer tester.Stop()

		// Inject 3 payloads
		p1 := schema.NewPayload("test")
		_ = p1.Set("id", "1")
		p2 := schema.NewPayload("test")
		_ = p2.Set("id", "2")
		p3 := schema.NewPayload("test")
		_ = p3.Set("id", "3")

		tester.InjectPayload(p1.ReadOnly())
		time.Sleep(5 * time.Millisecond)
		tester.InjectPayload(p2.ReadOnly())
		time.Sleep(5 * time.Millisecond)
		tester.InjectPayload(p3.ReadOnly())

		time.Sleep(5 * time.Millisecond) // Let data 1 start

		// Inject clear signal - this should bump epoch and cancel p1,
		// and cause p2 and p3 to be skipped in the readLoop due to epoch mismatch.
		tester.InjectSignal(schema.NewSignal(clearSignal).ReadOnly())

		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		// Only p1 might have started and been recorded.
		// p2 and p3 should be skipped.
		assert.Contains(t, processedIDs, "1")
		assert.NotContains(t, processedIDs, "2")
		assert.NotContains(t, processedIDs, "3")
		mu.Unlock()
	})

	t.Run("Concurrent Audio/Payload Tracks", func(t *testing.T) {
		mock := &MockSlowPlugin{}
		var payloadStart time.Time
		var audioArrived time.Time
		var mu sync.Mutex

		mock.OnPayloadFunc = func(ctx context.Context, flow Flow, payload schema.Payload) {
			mu.Lock()
			payloadStart = time.Now()
			mu.Unlock()
			time.Sleep(200 * time.Millisecond) // Slow payload
		}

		mock.OnAudioFunc = func(ctx context.Context, flow Flow, audio schema.Audio) {
			mu.Lock()
			audioArrived = time.Now()
			mu.Unlock()
		}

		// Both are buffered on DIFFERENT tracks
		wrapped := NewMultiTrackPlugin(mock,
			WithPayloadTrack(10, BlockIfFull),
			WithAudioTrack(10, BlockIfFull),
		)

		tester := NewPluginTester(t, wrapped).Start()
		defer tester.Stop()

		tester.InjectPayload(schema.NewPayload("test").ReadOnly())
		time.Sleep(50 * time.Millisecond) // Ensure payload starts

		tester.InjectAudio(schema.NewAudio(schema.AudioTTS, AudioSampleRate, AudioChannels).ReadOnly())

		time.Sleep(300 * time.Millisecond)

		mu.Lock()
		require.False(t, payloadStart.IsZero())
		require.False(t, audioArrived.IsZero())
		// In the NEW MultiTrackPlugin, Audio should arrive and finish BEFORE payload finishes
		// because they are on separate goroutines.
		assert.True(t, audioArrived.Before(payloadStart.Add(200*time.Millisecond)))
		mu.Unlock()
	})
}
