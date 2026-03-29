package asr

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

// UserTranscription is the standard output format for all ASR providers.
type UserTranscription struct {
	Text         string `json:"text"`              // The transcribed text
	Final        bool   `json:"final"`             // Whether this is the final result for this utterance
	Emotion      string `json:"emotion,omitempty"` // Optional emotion tag
	LanguageCode string `json:"language_code,omitempty"`
	Stable       bool   `json:"stable,omitempty"` // Whether the result is stable (usually for streaming)
}

// Provider defines the interface for specific ASR engine implementations.
type Provider interface {
	// Start begins the transcription session.
	Start(ctx context.Context) error
	Stop()
	// SendAudioData pushes raw audio bytes to the ASR engine.
	SendAudioData(data []byte, isLast bool) error
	// Shutdown gracefully closes the session and releases resources.
	Shutdown()
	// Connected returns whether the provider is currently connected to its service.
	Connected() bool
}

// BasePlugin provides common logic for ASR-type plugins.
// Specific ASR plugins should embed this and initialize the Provider field.
type BasePlugin struct {
	engine.BuiltinPlugin
	Provider Provider
	Ctx      context.Context
	Flow     engine.Flow
	paused   atomic.Bool

	retryAt time.Time
}

func (e *BasePlugin) OnStart(ctx context.Context, flow engine.Flow) error {
	e.Ctx = ctx
	e.Flow = flow
	return nil
}

func (e *BasePlugin) OnStop() {
	if e.Provider != nil {
		e.Provider.Shutdown()
	}
}

func (e *BasePlugin) OnPause(_ context.Context) {
	if !e.paused.CompareAndSwap(false, true) {
		return
	}
	if e.Provider != nil && e.Provider.Connected() {
		e.Provider.Stop()
	}
}

func (e *BasePlugin) OnResume(_ context.Context, _ engine.Flow) {
	e.paused.Store(false)
}

// OnAudio handles incoming audio frames. It manages the lifecycle of the ASR provider
// and ensures data is passed through properly.
func (e *BasePlugin) OnAudio(ctx context.Context, flow engine.Flow, audio schema.Audio) {
	if e.Provider == nil || e.paused.Load() {
		return
	}

	if !e.Provider.Connected() {
		now := time.Now()
		if now.Before(e.retryAt) {
			return
		}

		if err := e.Provider.Start(ctx); err != nil {
			e.retryAt = now.Add(3 * time.Second)
			slog.ErrorContext(ctx, "asr provider start failed, will retry after 3s", "error", err)
			return
		}
		e.retryAt = time.Time{}
	}

	// Send audio bytes to the specific provider implementation.
	if err := e.Provider.SendAudioData(audio.Bytes(), false); err != nil {
		slog.ErrorContext(ctx, "asr provider send audio data failed", "error", err)
	}
}

// HandleTranscription is a callback for providers to send results back into the workflow.
// It wraps the raw transcription in a standard schema.MutablePayload object.
func (e *BasePlugin) HandleTranscription(t *UserTranscription) {
	if t == nil || (t.Text == "" && !t.Final) {
		return
	}

	outPayload := schema.NewPayload(schema.PayloadASRResult)
	_ = outPayload.Set("text", t.Text)
	_ = outPayload.Set("is_final", t.Final)

	if t.Emotion != "" {
		_ = outPayload.Set("emotion", t.Emotion)
	}
	e.Flow.SendPayload(outPayload.ReadOnly())
}
