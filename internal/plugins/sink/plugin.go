package sink

import (
	"context"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	engine.BuiltinPlugin
}

func NewPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	return &Plugin{}
}

func (e *Plugin) OnSignal(ctx context.Context, flow engine.Flow, signal schema.Signal) {
	var packetType protocol.PacketType
	switch signal.Name() {
	case schema.SignalInterrupter:
		packetType = protocol.TypeInterrupter
	case schema.SignalUserSpeechStart:
		packetType = protocol.TypeUserSpeechStart
	case schema.SignalUserSpeechEnd:
		packetType = protocol.TypeUserSpeechEnd
	case schema.SignalAgentSpeechStart:
		packetType = protocol.TypeAgentSpeechStart
	case schema.SignalAgentSpeechEnd:
		packetType = protocol.TypeAgentSpeechEnd
	default:
		return
	}
	flow.Publish(packetType, nil)
}

func (e *Plugin) OnAudio(ctx context.Context, flow engine.Flow, audio schema.Audio) {
	flow.Publish(protocol.TypeAudio, audio.Bytes())
}

func (e *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	switch payload.Name() {
	case schema.PayloadCaption:
		sub := schema.GetAs[[]byte](payload, "caption")
		if len(sub) == 0 {
			return
		}
		flow.PublishFull(protocol.TypeCaption, protocol.EncodeJSON, sub)
	}
}

func init() {
	err := engine.RegisterPlugin[engine.EmptyPluginConfig](NewPlugin, engine.PluginMetadata{
		Name: "sink",
	})
	if err != nil {
		panic(err)
	}
}
