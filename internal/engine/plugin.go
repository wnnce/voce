package engine

import (
	"context"
	"fmt"

	"github.com/wnnce/voce/internal/schema"
)

type (
	PortMetadata struct {
		Type        EventType `json:"type"`
		Port        int       `json:"port"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
	}

	PluginMetadata struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Inputs      []Property     `json:"inputs"`
		Outputs     []Property     `json:"outputs"`
		Ports       []PortMetadata `json:"ports"`
	}

	// Plugin defines the processing logic for a node.
	// Each method represents a lifecycle stage or a data processing hook.
	Plugin interface {
		OnStart(ctx context.Context, flow Flow) error
		OnReady(ctx context.Context, flow Flow)
		OnPause(ctx context.Context)
		OnResume(ctx context.Context, flow Flow)
		OnStop()
		OnSignal(ctx context.Context, flow Flow, signal schema.Signal)
		OnPayload(ctx context.Context, flow Flow, payload schema.Payload)
		OnAudio(ctx context.Context, flow Flow, audio schema.Audio)
		OnVideo(ctx context.Context, flow Flow, video schema.Video)
	}
)

func (p *PortMetadata) Validate() error {
	if p.Port <= 0 {
		return fmt.Errorf("port index %d must be greater than 0 (0 is reserved for broadcast)", p.Port)
	}
	if p.Port >= MaxPortCount {
		return fmt.Errorf("port index %d exceeds maximum allowed port (%d)", p.Port, MaxPortCount-1)
	}
	return nil
}

type BuiltinPlugin struct{}

func (b *BuiltinPlugin) OnStart(_ context.Context, _ Flow) error {
	return nil
}

func (b *BuiltinPlugin) OnReady(_ context.Context, _ Flow) {}

func (b *BuiltinPlugin) OnPause(_ context.Context) {}

func (b *BuiltinPlugin) OnResume(_ context.Context, _ Flow) {}

func (b *BuiltinPlugin) OnStop() {}

func (b *BuiltinPlugin) OnSignal(_ context.Context, flow Flow, signal schema.Signal) {
	flow.SendSignal(signal)
}

func (b *BuiltinPlugin) OnPayload(_ context.Context, flow Flow, payload schema.Payload) {
	flow.SendPayload(payload)
}

func (b *BuiltinPlugin) OnAudio(_ context.Context, flow Flow, audio schema.Audio) {
	flow.SendAudio(audio)
}

func (b *BuiltinPlugin) OnVideo(_ context.Context, flow Flow, video schema.Video) {
	flow.SendVideo(video)
}
