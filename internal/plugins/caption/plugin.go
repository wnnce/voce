package caption

import (
	"context"
	"log/slog"
	"strings"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	engine.BuiltinPlugin
	ctx                   context.Context
	builder               *strings.Builder
	waitingUserFinal      bool
	pendingAssistantFinal bool
}

func NewPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	return &Plugin{
		builder: &strings.Builder{},
	}
}

func (e *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	slog.InfoContext(ctx, "Caption extension onStart")
	e.ctx = ctx
	return nil
}

func (e *Plugin) OnStop() {
	slog.InfoContext(e.ctx, "Caption extension onStop")
}

func (e *Plugin) OnSignal(ctx context.Context, flow engine.Flow, signal schema.Signal) {
	switch signal.Name() {
	case schema.SignalInterrupter, schema.SignalUserSpeechStart:
		e.builder.Reset()
		e.waitingUserFinal = false
		e.pendingAssistantFinal = false
	}
	flow.SendSignal(signal)
}

func (e *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	switch payload.Name() {
	case schema.PayloadASRResult:
		var sub schema.Caption
		_ = sub.BaseSchema.UnpackSchema(payload)
		sub.Text = schema.GetAs(payload, "text", "")
		sub.IsFinal = schema.GetAs(payload, "is_final", false)
		sub.Speaker = schema.GetAs(payload, "speaker", "")
		sub.Role = schema.RoleUser
		if !sub.IsFinal {
			e.waitingUserFinal = true
			e.sendCaption(ctx, flow, sub)
			return
		}
		e.waitingUserFinal = false
		e.sendCaption(ctx, flow, sub)
		e.flushPendingAssistantCaption(ctx, flow)
	case schema.PayloadLLMChunk:
		sub := e.makeAssistantCaption(payload)
		if e.waitingUserFinal {
			e.pendingAssistantFinal = e.pendingAssistantFinal || sub.IsFinal
			return
		}
		e.sendCaption(ctx, flow, sub)
		if sub.IsFinal {
			e.builder.Reset()
		}
	}
}

func (e *Plugin) makeAssistantCaption(payload schema.Payload) schema.Caption {
	sentence := schema.GetAs(payload, "sentence", "")
	sub := schema.Caption{
		IsFinal: schema.GetAs(payload, "is_final", false),
		Role:    schema.RoleAssistant,
	}
	_ = sub.BaseSchema.UnpackSchema(payload)
	e.builder.WriteString(sentence)
	sub.Text = e.builder.String()
	return sub
}

func (e *Plugin) flushPendingAssistantCaption(ctx context.Context, flow engine.Flow) {
	if e.builder.Len() == 0 {
		return
	}
	e.sendCaption(ctx, flow, schema.Caption{
		Text:    e.builder.String(),
		Role:    schema.RoleAssistant,
		IsFinal: e.pendingAssistantFinal,
	})
	if e.pendingAssistantFinal {
		e.builder.Reset()
	}
	e.pendingAssistantFinal = false
}

func (e *Plugin) sendCaption(ctx context.Context, flow engine.Flow, sub schema.Caption) {
	outputData := schema.NewPayload(schema.PayloadCaption)
	if err := outputData.Set("caption", sub); err != nil {
		slog.ErrorContext(ctx, "output payload set caption failed", "error", err)
		return
	}
	flow.SendPayload(outputData.ReadOnly())
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name: "caption",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "speaker", engine.TypeString, false).
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadCaption, "caption", engine.TypeObject, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
