package soniox_asr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/asr"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	asr.BasePlugin
	gws.BuiltinEventHandler

	socket    *gws.Conn
	cfg       *SonioxConfig
	connected atomic.Bool

	mu             sync.Mutex
	finalBuilder   strings.Builder
	currentSpeaker string
}

func NewPlugin(configure *SonioxConfig) engine.Plugin {
	p := &Plugin{
		cfg: configure,
	}
	p.Provider = p
	return p
}

func (p *Plugin) Start(ctx context.Context) error {
	if p.connected.Load() {
		return nil
	}

	addr := p.cfg.BaseUrl
	if addr == "" {
		addr = "wss://stt-rt.soniox.com/transcribe-websocket"
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+p.cfg.ApiKey)

	socket, resp, err := gws.NewClient(p, &gws.ClientOption{
		Addr:          addr,
		RequestHeader: header,
	})
	if err != nil {
		return fmt.Errorf("soniox_asr: connect: %w", err)
	}
	defer resp.Body.Close()

	initReq := clientConfig{
		Model:                    p.cfg.Model,
		AudioFormat:              "pcm_s16le",
		SampleRate:               16000,
		NumChannels:              1,
		EnableSpeakerDiarization: p.cfg.EnableSpeakerDiarization,
		EnableEndpointDetection:  p.cfg.EnableEndpointDetection,
	}
	if initReq.Model == "" {
		initReq.Model = "stt-rt-v6"
	}
	if p.cfg.LanguageHints != "" {
		for _, lang := range strings.Split(p.cfg.LanguageHints, ",") {
			trimmed := strings.TrimSpace(lang)
			if trimmed != "" {
				initReq.LanguageHints = append(initReq.LanguageHints, trimmed)
			}
		}
	}

	cfgBytes, err := sonic.Marshal(initReq)
	if err != nil {
		_ = socket.WriteMessage(gws.OpcodeCloseConnection, nil)
		return fmt.Errorf("soniox_asr: marshal init config: %w", err)
	}
	if err := socket.WriteMessage(gws.OpcodeText, cfgBytes); err != nil {
		_ = socket.WriteMessage(gws.OpcodeCloseConnection, nil)
		return fmt.Errorf("soniox_asr: send init config: %w", err)
	}

	p.socket = socket
	go socket.ReadLoop()

	p.connected.Store(true)
	slog.InfoContext(ctx, "soniox_asr: connected", "url", addr)
	return nil
}

func (p *Plugin) SendAudioData(data []byte, _ bool) error {
	if !p.connected.Load() {
		return errors.New("soniox_asr: not connected")
	}
	return p.socket.WriteMessage(gws.OpcodeBinary, data)
}

func (p *Plugin) Stop() {
	if !p.connected.CompareAndSwap(true, false) {
		return
	}
	if p.socket != nil {
		_ = p.socket.WriteMessage(gws.OpcodeCloseConnection, nil)
	}
}

func (p *Plugin) Shutdown() {
	p.Stop()
}

func (p *Plugin) Connected() bool {
	return p.connected.Load()
}

func (p *Plugin) OnClose(_ *gws.Conn, err error) {
	p.connected.Store(false)
	slog.InfoContext(p.Ctx, "soniox_asr: websocket closed", "err", err)
}

func (p *Plugin) OnPause(ctx context.Context) {
	p.BasePlugin.OnPause(ctx)
	p.mu.Lock()
	p.finalBuilder.Reset()
	p.currentSpeaker = ""
	p.mu.Unlock()
}

func (p *Plugin) OnMessage(_ *gws.Conn, message *gws.Message) {
	body := message.Bytes()
	defer message.Close()

	var resp serverResponse
	if err := sonic.Unmarshal(body, &resp); err != nil {
		slog.ErrorContext(p.Ctx, "soniox_asr: unmarshal response failed", "error", err, "raw", string(body))
		return
	}
	if resp.ErrorCode != 0 || resp.ErrorMessage != "" {
		slog.ErrorContext(p.Ctx, "soniox_asr: server error", "code", resp.ErrorCode, "msg", resp.ErrorMessage)
		return
	}
	p.handleResponse(&resp)
}

func (p *Plugin) handleResponse(resp *serverResponse) {
	if len(resp.Tokens) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var newFinalText strings.Builder
	var nonFinalText strings.Builder
	isEndpoint := false

	for _, tok := range resp.Tokens {
		if tok.Text == "<end>" {
			isEndpoint = true
			continue
		}
		if p.cfg.EnableSpeakerDiarization && tok.Speaker != "" {
			p.currentSpeaker = "speaker_" + tok.Speaker
		}
		if tok.IsFinal {
			newFinalText.WriteString(tok.Text)
		} else {
			nonFinalText.WriteString(tok.Text)
		}
	}

	if newFinalText.Len() > 0 {
		p.finalBuilder.WriteString(newFinalText.String())
	}

	finalStr := p.finalBuilder.String()
	nonFinalStr := nonFinalText.String()

	// If there are non-final tokens, this is an interim hypothesis
	if nonFinalStr != "" {
		fullText := finalStr + nonFinalStr
		data := &schema.Transcription{
			Text:    fullText,
			Stable:  false,
			Final:   false,
			Speaker: p.currentSpeaker,
		}
		p.HandleTranscription(data)
		return
	}

	// No non-final tokens. Check if we have final content to emit
	if finalStr != "" {
		if isEndpoint {
			data := &schema.Transcription{
				Text:    finalStr,
				Stable:  true,
				Final:   true,
				Speaker: p.currentSpeaker,
			}
			p.HandleTranscription(data)
			p.finalBuilder.Reset()
		} else if newFinalText.Len() > 0 {
			// A stable segment was finalized by ASR, but speech is still ongoing
			data := &schema.Transcription{
				Text:    finalStr,
				Stable:  true,
				Final:   false,
				Speaker: p.currentSpeaker,
			}
			p.HandleTranscription(data)
		}
	}
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "soniox_asr",
		Description: "Soniox real-time streaming ASR with semantic endpointing & diarization",
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "is_stable", engine.TypeBoolean, false).
			AddPayload(schema.PayloadASRResult, "speaker", engine.TypeString, false).
			Build(),
	}); err != nil {
		panic(err)
	}
}
