package elevenlabs_tts

import (
	"bytes"
	"context"
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

// outputFormat requests raw 16-bit PCM at 16 kHz (mono).
// This matches engine.AudioSampleRate (16 000 Hz) and engine.AudioChannels (1),
// so no resampling is needed.
const outputFormat = "pcm_16000"

// Plugin implements the ElevenLabs StreamSpeech TTS endpoint.
//
// For prosodic continuity, every text chunk sent to the API is cached in
// previousText.  On the next request that text is forwarded via the
// previous_text parameter so ElevenLabs can blend the boundary seamlessly.
// The buffer is cleared when a payload with is_final=true arrives.
type Plugin struct {
	engine.BuiltinPlugin
	streamer     *tts.AudioStreamer
	client       *http.Client
	cfg          *ElevenLabsConfig
	readBuf      []byte
	previousText string
}

func NewPlugin(cfg *ElevenLabsConfig) engine.Plugin {
	client := cfg.client
	if client == nil {
		client = http.DefaultClient
	}
	plg := &Plugin{
		cfg:     cfg,
		client:  client,
		readBuf: make([]byte, 4096),
	}
	return engine.NewMultiTrackPlugin(plg, engine.WithPayloadTrack(128, engine.BlockIfFull, schema.SignalInterrupter))
}

// ─── engine.Plugin lifecycle ──────────────────────────────────────────────────

func (p *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	p.streamer = tts.NewAudioStreamer(flow, engine.AudioSampleRate, engine.AudioChannels, 100*time.Millisecond)
	return nil
}

func (p *Plugin) OnSignal(_ context.Context, flow engine.Flow, signal schema.Signal) {
	if signal.Name() == schema.SignalInterrupter {
		// Interrupt: stop streaming and clear the previous-text buffer.
		p.streamer.Reset()
		p.previousText = ""
	}
	flow.SendSignal(signal)
}

// OnPayload handles each LLM text chunk.
//
// Flow:
//  1. Parse the payload; skip if empty and not final.
//  2. Call the ElevenLabs StreamSpeech endpoint, passing previousText for
//     prosodic continuity.
//  3. Accumulate the current text into previousText for the next request.
//  4. Stream the raw PCM bytes into the AudioStreamer.
//  5. On final, flush the streamer and reset previousText.
func (p *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	text, _, final, ok := tts.ParsePayload(payload)
	if !ok {
		return
	}

	defer func() {
		if final {
			// Flush any remaining audio and signal speech end.
			p.streamer.Write(ctx, nil, true)
			// Reset the previous-text buffer for the next utterance.
			p.previousText = ""
		}
	}()

	if text == "" || strings.TrimSpace(text) == "" {
		return
	}

	req, err := p.buildRequest(ctx, text)
	if err != nil {
		slog.ErrorContext(ctx, "elevenlabs_tts: build request failed", "error", err)
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "elevenlabs_tts: HTTP request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.ErrorContext(ctx, "elevenlabs_tts: API error",
			"status", resp.StatusCode, "body", string(body))
		return
	}

	p.previousText = text

	for {
		n, readErr := resp.Body.Read(p.readBuf)
		if n > 0 {
			p.streamer.Write(ctx, p.readBuf[:n], false)
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, context.Canceled) {
				slog.ErrorContext(ctx, "elevenlabs_tts: read response stream failed", "error", readErr)
			}
			break
		}
	}
}

// buildRequest constructs the HTTP POST to /v1/text-to-speech/{voice_id}/stream.
func (p *Plugin) buildRequest(ctx context.Context, text string) (*http.Request, error) {
	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.elevenlabs.io"
	}
	endpoint := fmt.Sprintf("%s/v1/text-to-speech/%s/stream", baseURL, p.cfg.VoiceID)

	body := &streamRequest{
		Text:         text,
		ModelID:      p.cfg.ModelID,
		OutputFormat: outputFormat,
	}

	// Attach previous_text for prosodic continuity if we have buffered text.
	if p.previousText != "" {
		body.PreviousText = p.previousText
	}

	// Attach voice settings only when explicitly configured.
	if p.cfg.Stability > 0 || p.cfg.SimilarityBoost > 0 {
		body.VoiceSettings = &voiceSettings{
			Stability:       p.cfg.Stability,
			SimilarityBoost: p.cfg.SimilarityBoost,
		}
	}

	payload, err := sonic.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", p.cfg.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/pcm")
	return req, nil
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "elevenlabs_tts",
		Description: "ElevenLabs StreamSpeech TTS with prosodic context (previous_text)",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
