package mllm

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic"
)

const (
	ProviderQwen = "qwen"
)

type ToolCall struct {
	CallID    string
	Name      string
	Arguments string
	Metadata  map[string]string
}

type ToolCallAnswer struct {
	ToolCall
	Result string
}

type RealtimeProvider interface {
	Connect(ctx context.Context) error
	Listen(listener RealtimeListener)
	Name() string
	SendAudio(data []byte) error
	SendText(text string) error
	SendImage(data []byte) error
	SendToolCallAnswer(answer ToolCallAnswer) error
	Interrupter() error
	Connected() bool
	Close()
}

type RealtimeListener interface {
	OnAudioDelta(data []byte)
	// OnAudioDone is called when the model finishes sending audio for a response.
	OnAudioDone()
	// OnTranscriptDelta receives incremental assistant response text.
	OnTranscriptDelta(text string)
	// OnTranscriptDone receives the final assistant response transcript.
	OnTranscriptDone(text string)
	// OnInputTranscriptionDelta receives the final user speech transcription.
	OnInputTranscriptionDelta(text string)
	OnInputTranscriptionDone(text string)
	// OnSpeechStarted is called when server VAD detects user speech.
	OnSpeechStarted()
	// OnSpeechStopped is called when server VAD detects end of user speech.
	OnSpeechStopped()
	OnToolCall(call ToolCall)
	// OnError is called when the server sends an error event.
	OnError(code, message string)
}

func NewRealtimeProvider(ctx context.Context, cfg RealtimeConfig) (RealtimeProvider, error) {
	switch cfg.Provider {
	case ProviderQwen:
		var properties QwenConfig
		if err := sonic.Unmarshal(cfg.Properties, &properties); err != nil {
			return nil, err
		}
		return NewQwenRealtimeProvider(ctx, properties)
	}
	return nil, fmt.Errorf("mllm: unsupported realtime provider %q", cfg.Provider)
}
