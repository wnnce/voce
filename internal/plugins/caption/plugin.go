package caption

import (
	"context"
	"log/slog"
	"strings"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

const (
	roleUser      = "user"
	roleAssistant = "assistant"
)

type Caption struct {
	Text    string `json:"text"`
	Role    string `json:"role"`
	IsFinal bool   `json:"is_final"`
}

type Plugin struct {
	engine.BuiltinPlugin
	ctx     context.Context
	builder *strings.Builder
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
	if signal.Name() == schema.SignalInterrupter {
		e.builder.Reset()
	}
	flow.SendSignal(signal)
}

func (e *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	var sub Caption
	switch payload.Name() {
	case schema.PayloadASRResult:
		sub.Text = schema.GetAs(payload, "text", "")
		sub.IsFinal = schema.GetAs(payload, "is_final", false)
		sub.Role = roleUser
	case schema.PayloadLLMChunk:
		sentence := schema.GetAs(payload, "sentence", "")
		sub.IsFinal = schema.GetAs(payload, "is_final", false)
		sub.Role = roleAssistant
		e.builder.WriteString(sentence)
		sub.Text = e.builder.String()
		if sub.IsFinal {
			e.builder.Reset()
		}
	}
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
