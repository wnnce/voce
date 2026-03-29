package minimax_tts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/tts"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	engine.BuiltinPlugin
	config   *MinimaxConfig
	options  *MinimaxOptions
	streamer *tts.AudioStreamer
}

func NewPlugin(cfg *MinimaxConfig) engine.Plugin {
	plugin := &Plugin{
		config: cfg,
	}
	return engine.NewMultiTrackPlugin(plugin, engine.WithPayloadTrack(
		128, engine.BlockIfFull, schema.SignalInterrupter,
	))
}

func (p *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	if p.config.Token == "" || p.config.BaseUrl == "" || p.config.VoiceID == "" {
		return fmt.Errorf("minimax token base_url voice_id is required")
	}
	if p.config.Model == "" {
		p.config.Model = "speech-01-turbo"
	}
	p.options = &MinimaxOptions{
		Model:  p.config.Model,
		Stream: true,
		StreamOptions: &StreamOptions{
			ExcludeAggregatedAudio: true,
		},
		AudioSetting: &AudioSetting{
			SampleRate: engine.AudioSampleRate,
			Channel:    engine.AudioChannels,
			Format:     "pcm",
		},
		VoiceSetting: &VoiceSetting{
			VoiceId: p.config.VoiceID,
		},
	}
	p.streamer = tts.NewAudioStreamer(flow, engine.AudioSampleRate, engine.AudioChannels, 100*time.Millisecond)
	return nil
}

func (p *Plugin) OnSignal(ctx context.Context, flow engine.Flow, signal schema.Signal) {
	if signal.Name() == schema.SignalInterrupter {
		p.streamer.Reset()
	}
	flow.SendSignal(signal)
}

func (p *Plugin) OnPayload(ctx context.Context, flow engine.Flow, data schema.Payload) {
	text, emotion, final, ok := tts.ParsePayload(data)
	if !ok {
		return
	}
	if err := p.process(ctx, text, emotion, final); err != nil && !errors.Is(err, context.Canceled) {
		slog.ErrorContext(ctx, "minimax process failed", "error", err)
	}
}

func (p *Plugin) process(ctx context.Context, text, emotion string, final bool) error {
	defer func() {
		if final {
			p.streamer.Write(ctx, nil, true)
		}
	}()
	if text == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	request := p.newRequest(ctx, text, emotion)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("request error, status: %d", response.StatusCode)
	}
	return p.handlerResponseBody(ctx, response.Body)
}

func (p *Plugin) handlerResponseBody(ctx context.Context, r io.Reader) error {
	reader := bufio.NewReader(r)
	for {
		line, readerError := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)

		if len(line) > 0 && bytes.HasPrefix(line, []byte("data: ")) {
			p.processSSELine(ctx, line[6:])
		}

		if readerError == nil {
			continue
		}
		if errors.Is(readerError, io.EOF) {
			break
		}
		if errors.Is(readerError, context.Canceled) {
			return nil
		}
		slog.ErrorContext(ctx, "minimax sse stream failed", "error", readerError)
		return readerError
	}
	slog.InfoContext(ctx, "minimax sse stream done")
	return nil
}

func (p *Plugin) processSSELine(ctx context.Context, data []byte) {
	result := &MinimaxResponse{}
	if err := sonic.Unmarshal(data, result); err != nil {
		slog.ErrorContext(ctx, "minimax unmarshal failed", "error", err)
		return
	}

	if result.BaseResp.StatusCode != 0 {
		slog.ErrorContext(ctx, "minimax request failed", "code", result.BaseResp.StatusCode, "msg", result.BaseResp.StatusMsg)
		return
	}

	if result.Data == nil || result.Data.Audio == "" {
		return
	}

	audioChunk, err := hex.DecodeString(result.Data.Audio)
	if err != nil {
		slog.ErrorContext(ctx, "minimax audio decode failed", "error", err)
		return
	}

	p.streamer.Write(ctx, audioChunk, false)
}

func (p *Plugin) newRequest(ctx context.Context, text, emotion string) *http.Request {
	body := &MinimaxRequest{
		MinimaxOptions: *p.options,
		Text:           text,
	}
	if strings.TrimSpace(emotion) != "" {
		body.VoiceSetting.Emotion = emotion
	}
	jsonBody, _ := sonic.Marshal(body)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseUrl, bytes.NewReader(jsonBody))
	request.Header.Set("Authorization", "Bearer "+p.config.Token)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name: "minimax_tts",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
