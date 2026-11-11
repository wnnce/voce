package interrupter

import (
	"context"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	engine.BuiltinPlugin
	speaking bool
	turnId   uint64
}

func NewPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	return &Plugin{}
}

func (e *Plugin) OnPayload(_ context.Context, flow engine.Flow, data schema.Payload) {
	isFinal := schema.GetAs(data, "is_final", false)
	if !e.speaking {
		e.speaking = true
		e.turnId += 1

		interrupter := schema.NewSignal(schema.SignalInterrupter)
		_ = interrupter.Set("turn_id", e.turnId)
		flow.SendSignal(interrupter.ReadOnly())

		started := schema.NewSignal(schema.SignalUserSpeechStart)
		_ = started.Set("turn_id", e.turnId)
		flow.SendSignal(started.ReadOnly())
	}

	if isFinal {
		e.speaking = false
		ended := schema.NewSignal(schema.SignalUserSpeechEnd)
		_ = ended.Set("turn_id", e.turnId)
		flow.SendSignal(ended.ReadOnly())
	}

	transcription := data.Mutable()
	_ = transcription.Set("turn_id", e.turnId)
	flow.SendPayload(transcription.ReadOnly())
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name: "interrupter",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "emotion", engine.TypeString, false).
			Build(),
		Outputs: engine.NewPropertyBuilder().
			AddSignalEvent(schema.SignalInterrupter).
			AddSignalEvent(schema.SignalUserSpeechStart).
			AddSignalEvent(schema.SignalUserSpeechEnd).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "emotion", engine.TypeString, false).
			AddPayload(schema.PayloadASRResult, "turn_id", engine.TypeInteger, false).
			Build(),
	}); err != nil {
		panic(err)
	}
}
