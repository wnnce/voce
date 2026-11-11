package translator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	engine.BuiltinPlugin
	ctx           context.Context
	cancel        context.CancelFunc
	cfg           TranslationConfig
	fastProvider  Provider
	finalProvider Provider

	speakersMu sync.Mutex
	speakers   map[string]*SpeakerContext
	wg         sync.WaitGroup
}

func NewPlugin(cfg *TranslationConfig) engine.Plugin {
	return &Plugin{
		cfg:      *cfg,
		speakers: make(map[string]*SpeakerContext),
	}
}

func (p *Plugin) OnStart(ctx context.Context, _ engine.Flow) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	fast, err := NewProvider(p.ctx, p.cfg.FastProvider)
	if err != nil {
		return fmt.Errorf("translator: failed to init fast provider: %w", err)
	}
	p.fastProvider = fast

	if p.cfg.FinalProvider != nil && p.cfg.FinalProvider.Type != "" {
		final, err := NewProvider(p.ctx, *p.cfg.FinalProvider)
		if err != nil {
			return fmt.Errorf("translator: failed to init final provider: %w", err)
		}
		p.finalProvider = final
	} else {
		p.finalProvider = fast
	}

	slog.InfoContext(ctx, "translator plugin started",
		"fast_provider", p.fastProvider.Name(),
		"final_provider", p.finalProvider.Name(),
		"source_lang", p.cfg.SourceLang,
		"target_lang", p.cfg.TargetLang)
	return nil
}

func (p *Plugin) OnStop() {
	if p.cancel != nil {
		p.cancel()
	}

	p.speakersMu.Lock()
	for _, sc := range p.speakers {
		sc.Reset()
	}
	p.speakersMu.Unlock()

	p.wg.Wait()

	if p.fastProvider != nil {
		_ = p.fastProvider.Close()
	}
	if p.finalProvider != nil && p.finalProvider != p.fastProvider {
		_ = p.finalProvider.Close()
	}
}

func (p *Plugin) OnSignal(_ context.Context, flow engine.Flow, signal schema.Signal) {
	switch signal.Name() {
	case schema.SignalInterrupter, schema.SignalUserSpeechStart:
		p.speakersMu.Lock()
		for _, sc := range p.speakers {
			sc.Reset()
		}
		p.speakersMu.Unlock()
	}
	flow.SendSignal(signal)
}

func (p *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	if payload.Name() != schema.PayloadASRResult {
		return
	}

	var trans schema.Transcription
	if err := payload.Unpack(&trans); err != nil {
		return
	}

	speaker := trans.Speaker
	if speaker == "" {
		speaker = "default"
	}

	p.speakersMu.Lock()
	sc, ok := p.speakers[speaker]
	if !ok {
		sc = NewSpeakerContext(speaker)
		p.speakers[speaker] = sc
	}
	p.speakersMu.Unlock()

	sc.HandleASR(ctx, flow, &trans, p)
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "translator",
		Description: "Real-time bilingual simultaneous translation with draft-and-revise",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "is_stable", engine.TypeBoolean, false).
			AddPayload(schema.PayloadASRResult, "speaker", engine.TypeString, false).
			AddPayload(schema.PayloadASRResult, "turn_id", engine.TypeInteger, false).
			Build(),
		Outputs: engine.NewPropertyBuilder().
			AddSignalEvent(schema.SignalInterrupter).
			AddSignalEvent(schema.SignalUserSpeechStart).
			AddSignalEvent(schema.SignalUserSpeechEnd).
			AddPayload(schema.PayloadCaption, "text", engine.TypeString, true).
			AddPayload(schema.PayloadCaption, "translation", engine.TypeString, true).
			AddPayload(schema.PayloadCaption, "role", engine.TypeString, true).
			AddPayload(schema.PayloadCaption, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadCaption, "speaker", engine.TypeString, false).
			AddPayload(schema.PayloadCaption, "turn_id", engine.TypeInteger, false).
			Build(),
	}); err != nil {
		panic(err)
	}
}
