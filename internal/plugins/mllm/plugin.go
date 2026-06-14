package mllm

import (
	"context"
	"log/slog"
	"time"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/tts"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/audioproc"
)

type Plugin struct {
	engine.BuiltinPlugin
	ctx      context.Context
	flow     engine.Flow
	cfg      RealtimeConfig
	provider RealtimeProvider
	streamer *tts.AudioStreamer

	inputResampler  *audioproc.Resampler
	outputResampler *audioproc.Resampler
}

func NewPlugin(cfg *RealtimeConfig) engine.Plugin {
	return &Plugin{
		cfg: *cfg,
	}
}

func (p *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	provider, err := NewRealtimeProvider(ctx, p.cfg)
	if err != nil {
		return err
	}
	if err = provider.Connect(ctx); err != nil {
		return err
	}
	var opts []tts.AudioStreamerOption
	if p.cfg.OutputSampleRate != engine.AudioSampleRate || p.cfg.OutputChannels != engine.AudioChannels {
		p.outputResampler, err = audioproc.NewResampler(
			p.cfg.OutputSampleRate,
			p.cfg.OutputChannels,
			engine.AudioSampleRate,
			engine.AudioChannels,
		)
		if err != nil {
			return err
		}
		opts = append(opts, tts.WithResampler(p.outputResampler))
	}
	p.streamer = tts.NewAudioStreamer(
		flow,
		engine.AudioSampleRate,
		engine.AudioChannels,
		100*time.Millisecond,
		opts...,
	)
	provider.Listen(p)
	p.provider = provider
	p.ctx = ctx
	p.flow = flow
	return nil
}

func (p *Plugin) OnStop() {
	if p.inputResampler != nil {
		p.inputResampler.Close()
		p.inputResampler = nil
	}
	if p.outputResampler != nil {
		p.outputResampler.Close()
		p.outputResampler = nil
	}
	if p.provider == nil || !p.provider.Connected() {
		return
	}
	p.provider.Close()
}

func (p *Plugin) OnAudio(ctx context.Context, flow engine.Flow, audio schema.Audio) {
	if p.provider == nil || !p.provider.Connected() {
		return
	}
	data := audio.Bytes()
	if p.cfg.InputSampleRate != 0 && p.cfg.InputSampleRate != audio.SampleRate() {
		resampled, err := p.resampleInputAudio(audio)
		if err != nil {
			slog.ErrorContext(ctx, "mllm resample input audio failed", "error", err)
			return
		}
		data = resampled
	}
	if err := p.provider.SendAudio(data); err != nil {
		slog.ErrorContext(ctx, "mllm send audio failed", "error", err)
	}
}

func (p *Plugin) resampleInputAudio(audio schema.Audio) ([]byte, error) {
	if p.inputResampler == nil {
		resampler, err := audioproc.NewResampler(
			audio.SampleRate(),
			audio.Channels(),
			p.cfg.InputSampleRate,
			audio.Channels(),
		)
		if err != nil {
			return nil, err
		}
		p.inputResampler = resampler
	}
	return p.inputResampler.Resample(audio.Bytes())
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "mllm",
		Description: "realtime large language model",
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "emotion", engine.TypeString, false).
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
