package engine

import (
	"context"
	"testing"
	"time"

	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

const (
	DefaultActivityTimeout = 10 * time.Second
	ActivityBufferSize     = 1024
)

// PluginTester provides a harness for unit testing individual plugins in isolation.
// It uses a MockFlow to capture outputs and track plugin activity.
type PluginTester struct {
	t      *testing.T
	ext    Plugin
	mock   *MockFlow
	ctx    context.Context
	cancel context.CancelFunc

	activity     chan struct{} // channel for tracking plugin asycn activity
	activityWait time.Duration // timeout for activity idle wait
}

func NewPluginTester(t *testing.T, ext Plugin) *PluginTester {
	ctx, cancel := context.WithCancel(context.Background())
	activity := make(chan struct{}, ActivityBufferSize)

	tester := &PluginTester{
		t:            t,
		ext:          ext,
		mock:         &MockFlow{},
		ctx:          ctx,
		cancel:       cancel,
		activity:     activity,
		activityWait: DefaultActivityTimeout,
	}

	tester.mock.onActivity = tester.pingActivity
	return tester
}

func (et *PluginTester) Start() *PluginTester {
	if err := et.ext.OnStart(et.ctx, et.mock); err != nil {
		et.t.Fatalf("Plugin OnStart failed: %v", err)
	}
	et.ext.OnReady(et.ctx, et.mock)
	return et
}

func (et *PluginTester) Stop() {
	et.ext.OnStop()
	et.cancel()
}

func (et *PluginTester) Done() {
	et.cancel()
}

// Wait blocks until the plugin stops emitting activity for the specified duration.
// This is useful for testing asynchronous plugins that process data in the background.
func (et *PluginTester) Wait(timeout ...time.Duration) {
	deadline := et.activityWait
	if len(timeout) > 0 {
		deadline = timeout[0]
	}

	timer := time.NewTimer(deadline)
	defer timer.Stop()

	for {
		select {
		case <-et.ctx.Done():
			return
		case <-et.activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(deadline)
		case <-timer.C:
			et.t.Errorf("Plugin activity timed out after %v", deadline)
			et.cancel()
			return
		}
	}
}

func (et *PluginTester) pingActivity() {
	select {
	case et.activity <- struct{}{}:
	default:
	}
}

func (et *PluginTester) InjectSignal(signal schema.Signal) *PluginTester {
	et.t.Helper()
	et.pingActivity() // Keep pingActivity for activity tracking
	et.ext.OnSignal(et.ctx, et.mock, signal)
	return et
}

func (et *PluginTester) InjectPayload(payload schema.Payload) *PluginTester {
	et.t.Helper()
	et.pingActivity() // Keep pingActivity for activity tracking
	et.ext.OnPayload(et.ctx, et.mock, payload)
	return et
}

func (et *PluginTester) InjectAudio(audio schema.Audio) *PluginTester {
	et.pingActivity()
	et.ext.OnAudio(et.ctx, et.mock, audio)
	return et
}

func (et *PluginTester) InjectVideo(video schema.Video) *PluginTester {
	et.pingActivity()
	et.ext.OnVideo(et.ctx, et.mock, video)
	return et
}

// On* methods allow tests to register hooks to intercept specific output types from the plugin.
func (et *PluginTester) OnSignal(cb func(int, schema.Signal)) *PluginTester {
	et.mock.OnSignalHook = cb
	return et
}

func (et *PluginTester) OnPayload(cb func(int, schema.Payload)) *PluginTester {
	et.mock.OnPayloadHook = cb
	return et
}

func (et *PluginTester) OnAudio(cb func(int, schema.Audio)) *PluginTester {
	et.mock.OnAudioHook = cb
	return et
}

func (et *PluginTester) OnVideo(cb func(int, schema.Video)) *PluginTester {
	et.mock.OnVideoHook = cb
	return et
}

func (et *PluginTester) OnPublish(cb func(protocol.PacketType, []byte)) *PluginTester {
	et.mock.OnPublishHook = cb
	return et
}

func (et *PluginTester) OnPublishFull(cb func(protocol.PacketType, protocol.PacketEncode, []byte)) *PluginTester {
	et.mock.OnPublishFullHook = cb
	return et
}

func (et *PluginTester) Context() context.Context {
	return et.ctx
}
