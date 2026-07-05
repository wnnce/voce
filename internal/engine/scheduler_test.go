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

type schedulerMockPlugin struct {
	BuiltinPlugin
	onSignal func(ctx context.Context, flow Flow, signal schema.Signal)
	onAudio  func(ctx context.Context, flow Flow, audio schema.Audio)
}

func (m *schedulerMockPlugin) OnSignal(ctx context.Context, flow Flow, signal schema.Signal) schema.Result {
	if m.onSignal != nil {
		m.onSignal(ctx, flow, signal)
	}
	return nil
}

func (m *schedulerMockPlugin) OnAudio(ctx context.Context, flow Flow, audio schema.Audio) {
	if m.onAudio != nil {
		m.onAudio(ctx, flow, audio)
	}
}

func TestScheduler_DispatchAndPriority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewScheduler(ctx, 1, 100)

	var (
		mu        sync.Mutex
		processed []string
		a1Started = make(chan struct{})
	)

	plg := &schedulerMockPlugin{
		onSignal: func(ctx context.Context, flow Flow, signal schema.Signal) {
			mu.Lock()
			processed = append(processed, "signal-"+signal.Name())
			mu.Unlock()
		},
		onAudio: func(ctx context.Context, flow Flow, audio schema.Audio) {
			mu.Lock()
			processed = append(processed, "audio-"+audio.Name())
			mu.Unlock()
			if audio.Name() == "1" {
				close(a1Started)
			}
			time.Sleep(20 * time.Millisecond)
		},
	}

	n := newSchedulerNode(ctx, "test-node", plg, s)
	n.running.Store(true)

	s.Submit(n, schema.NewAudio("1", AudioSampleRate, AudioChannels).ReadOnly())

	select {
	case <-a1Started:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for audio 1")
	}

	s.Submit(n, schema.NewAudio("2", AudioSampleRate, AudioChannels).ReadOnly())
	s.Submit(n, schema.NewAudio("3", AudioSampleRate, AudioChannels).ReadOnly())
	s.Submit(n, schema.NewSignal("interrupt").ReadOnly())

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(processed) == 4
	}, 1*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, processed, 4)
	assert.Equal(t, []string{"audio-1", "signal-interrupt", "audio-2", "audio-3"}, processed)
}

func TestScheduler_BackpressureDrop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewScheduler(ctx, 1, 2)

	var (
		audioCalled atomic.Int32
		wg          sync.WaitGroup
		started     = make(chan struct{}, 1)
	)

	plg := &schedulerMockPlugin{
		onAudio: func(ctx context.Context, flow Flow, audio schema.Audio) {
			audioCalled.Add(1)
			wg.Done()
			select {
			case started <- struct{}{}:
			default:
			}
			time.Sleep(50 * time.Millisecond)
		},
	}

	n := newSchedulerNode(ctx, "test-node", plg, s)
	n.running.Store(true)

	wg.Add(3)
	s.Submit(n, schema.NewAudio("1", AudioSampleRate, AudioChannels).ReadOnly())

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for task 1")
	}

	s.Submit(n, schema.NewAudio("2", AudioSampleRate, AudioChannels).ReadOnly())
	s.Submit(n, schema.NewAudio("3", AudioSampleRate, AudioChannels).ReadOnly())
	s.Submit(n, schema.NewAudio("4", AudioSampleRate, AudioChannels).ReadOnly())

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for scheduler")
	}

	assert.Equal(t, int32(3), audioCalled.Load())
}
