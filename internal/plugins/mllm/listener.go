package mllm

import (
	"log/slog"

	"github.com/wnnce/voce/internal/plugins/base/asr"
	"github.com/wnnce/voce/internal/plugins/base/llm"
	"github.com/wnnce/voce/internal/schema"
)

func (p *Plugin) OnAudioDelta(data []byte) {
	p.streamer.Write(p.ctx, data, false)
}

func (p *Plugin) OnAudioDone() {
	p.streamer.Write(p.ctx, nil, true)
}

func (p *Plugin) OnTranscriptDelta(text string) {
	payload := schema.NewPayload(schema.PayloadLLMChunk)
	_ = payload.Pack(&llm.Chunk{
		Sentence: text,
		IsFinal:  false,
	})
	p.flow.SendPayload(payload.ReadOnly())
}

func (p *Plugin) OnTranscriptDone(_ string) {
	payload := schema.NewPayload(schema.PayloadLLMChunk)
	_ = payload.Pack(&llm.Chunk{
		Sentence: "",
		IsFinal:  true,
	})
	p.flow.SendPayload(payload.ReadOnly())
}

func (p *Plugin) OnInputTranscriptionDelta(text string) {
	payload := schema.NewPayload(schema.PayloadASRResult)
	_ = payload.Pack(&asr.UserTranscription{
		Text:  text,
		Final: false,
	})
	p.flow.SendPayload(payload.ReadOnly())
}

func (p *Plugin) OnInputTranscriptionDone(text string) {
	payload := schema.NewPayload(schema.PayloadASRResult)
	_ = payload.Pack(&asr.UserTranscription{
		Text:  text,
		Final: true,
	})
	p.flow.SendPayload(payload.ReadOnly())
}

func (p *Plugin) OnSpeechStarted() {
	signal := schema.NewSignal(schema.SignalInterrupter)
	p.flow.SendSignal(signal.ReadOnly())
	signal = schema.NewSignal(schema.SignalUserSpeechStart)
	p.flow.SendSignal(signal.ReadOnly())
}

func (p *Plugin) OnSpeechStopped() {
	signal := schema.NewSignal(schema.SignalUserSpeechEnd)
	p.flow.SendSignal(signal.ReadOnly())
}

func (p *Plugin) OnToolCall(call ToolCall) {
	slog.InfoContext(p.ctx, "mllm listener tool call",
		slog.String("call_id", call.CallID),
		slog.String("name", call.Name),
		slog.String("arguments", call.Arguments),
	)
}

func (p *Plugin) OnError(code, message string) {
	slog.ErrorContext(p.ctx, "mllm listener error",
		slog.String("code", code),
		slog.String("message", message),
	)
}
