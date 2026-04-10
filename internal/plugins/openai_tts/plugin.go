package openai_tts

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/tts"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/audioproc"
)

type OpenAIConfig struct {
	ApiKey  string  `json:"api_key" jsonschema:"title=API Key,description=OpenAI Api Key"`
	BaseURL string  `json:"base_url" jsonschema:"title=Base URL,default=https://api.openai.com/v1"`
	Model   string  `json:"model" jsonschema:"title=Model,default=tts-1"`
	Voice   string  `json:"voice" jsonschema:"title=Voice,default=alloy"`
	Speed   float64 `json:"speed" jsonschema:"title=Speed,default=1.0,minimum=0.25,maximum=4.0"`
	client  *http.Client
}

func (c *OpenAIConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(c)
}

func (c *OpenAIConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, c)
}

type TTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

type Plugin struct {
	engine.BuiltinPlugin
	streamer  *tts.AudioStreamer
	client    *http.Client
	cfg       *OpenAIConfig
	buffer    []byte
	resampler *audioproc.Resampler
}

func NewPlugin(cfg *OpenAIConfig) engine.Plugin {
	client := cfg.client
	if client == nil {
		client = http.DefaultClient
	}
	plg := &Plugin{
		client: client,
		cfg:    cfg,
		buffer: make([]byte, 4096),
	}
	return engine.NewMultiTrackPlugin(plg, engine.WithPayloadTrack(128, engine.BlockIfFull, schema.SignalInterrupter))
}

func (p *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	var err error
	p.resampler, err = audioproc.NewResampler(24000, 1, engine.AudioSampleRate, engine.AudioChannels)
	if err != nil {
		return err
	}
	p.streamer = tts.NewAudioStreamer(flow, engine.AudioSampleRate, engine.AudioChannels, 100*time.Millisecond,
		tts.WithResampler(p.resampler))
	return nil
}

func (p *Plugin) OnStop() {
	if p.resampler != nil {
		p.resampler.Close()
	}
}

func (p *Plugin) OnSignal(ctx context.Context, flow engine.Flow, signal schema.Signal) {
	if signal.Name() == schema.SignalInterrupter {
		p.streamer.Reset()
	}
	flow.SendSignal(signal)
}

func (p *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	text, _, final, ok := tts.ParsePayload(payload)
	if !ok {
		return
	}
	defer func() {
		if final {
			p.streamer.Write(ctx, nil, true)
		}
	}()
	if text == "" || strings.TrimSpace(text) == "" {
		return
	}
	request := p.createRequest(ctx, text)
	response, err := p.client.Do(request)
	if err != nil {
		slog.ErrorContext(ctx, "request openai tts failed", "error", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(response.Body)
		slog.ErrorContext(ctx, "openai tts api error", "status", response.StatusCode, "body", string(msg))
		return
	}
	for {
		read, err := response.Body.Read(p.buffer)
		if read > 0 {
			p.streamer.Write(ctx, p.buffer[:read], false)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
				slog.ErrorContext(ctx, "reader openai tts response stream failed", "error", err)
			}
			break
		}
	}
}

func (p *Plugin) createRequest(ctx context.Context, text string) *http.Request {
	rest := &TTSRequest{
		Model:          p.cfg.Model,
		Input:          text,
		Voice:          p.cfg.Voice,
		ResponseFormat: engine.AudioFormat,
		Speed:          p.cfg.Speed,
	}
	payload, _ := sonic.Marshal(rest)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL, bytes.NewBuffer(payload))
	request.Header.Set("Authorization", "Bearer "+p.cfg.ApiKey)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name: "openai_tts",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
