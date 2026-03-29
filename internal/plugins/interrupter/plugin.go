package interrupter

import (
	"context"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	engine.BuiltinPlugin
	speaking bool
}

func NewPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	return &Plugin{}
}

func (e *Plugin) OnPayload(_ context.Context, flow engine.Flow, data schema.Payload) {
	isFinal := schema.GetAs(data, "is_final", false)
	if !isFinal && !e.speaking {
		e.speaking = true
		flow.SendSignal(schema.NewSignal(schema.SignalInterrupter).ReadOnly())
		flow.SendSignal(schema.NewSignal(schema.SignalUserSpeechStart).ReadOnly())
	} else if isFinal {
		e.speaking = false
		flow.SendSignal(schema.NewSignal(schema.SignalUserSpeechEnd).ReadOnly())
	}
	flow.SendPayload(data)
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
			Build(),
	}); err != nil {
		panic(err)
	}
}
