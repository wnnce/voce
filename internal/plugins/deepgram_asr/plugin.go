package deepgram_asr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/asr"
	"github.com/wnnce/voce/internal/schema"
)

// closeStreamPayload is the graceful close control message.
var closeStreamPayload = []byte(`{"type":"CloseStream"}`)

// Plugin implements asr.Provider for Deepgram's live-streaming WebSocket API.
//
// Lifecycle:
//
//	OnAudio → Start (if not connected) → SendAudioData (loop)
//	OnPause → Stop (closes WebSocket; KeepAlive goroutine exits)
//	OnResume → next OnAudio will call Start again
//	OnStop/Shutdown → Stop
type Plugin struct {
	asr.BasePlugin
	gws.BuiltinEventHandler

	socket    *gws.Conn
	cfg       *DeepgramConfig
	connected atomic.Bool
}

func NewPlugin(configure *DeepgramConfig) engine.Plugin {
	p := &Plugin{
		cfg: configure,
	}
	p.Provider = p
	return p
}

// Start establishes the WebSocket connection to Deepgram and launches the
// KeepAlive goroutine.
func (p *Plugin) Start(ctx context.Context) error {
	if p.connected.Load() {
		return nil
	}

	wsURL, err := p.buildURL()
	if err != nil {
		return fmt.Errorf("deepgram_asr: build URL: %w", err)
	}

	header := http.Header{}
	header.Set("Authorization", "Token "+p.cfg.ApiKey)

	socket, resp, err := gws.NewClient(p, &gws.ClientOption{
		Addr:          wsURL,
		RequestHeader: header,
	})
	if err != nil {
		return fmt.Errorf("deepgram_asr: connect: %w", err)
	}
	defer resp.Body.Close()

	p.socket = socket
	go socket.ReadLoop()

	p.connected.Store(true)
	slog.InfoContext(ctx, "deepgram_asr: connected", "url", wsURL)
	return nil
}

// SendAudioData forwards raw PCM bytes as a binary WebSocket frame.
func (p *Plugin) SendAudioData(data []byte, _ bool) error {
	if !p.connected.Load() {
		return errors.New("deepgram_asr: not connected")
	}
	return p.socket.WriteMessage(gws.OpcodeBinary, data)
}

// Stop sends CloseStream and marks as disconnected.
// The actual WebSocket close is handled by Deepgram responding with a close frame.
func (p *Plugin) Stop() {
	if !p.connected.CompareAndSwap(true, false) {
		return
	}
	if p.socket != nil {
		// Gracefully tell Deepgram we are done; ignore write errors here.
		_ = p.socket.WriteMessage(gws.OpcodeText, closeStreamPayload)
	}
}

// Shutdown is called by the engine on workflow termination.
func (p *Plugin) Shutdown() {
	p.Stop()
}

// Connected reports whether the WebSocket connection is currently active.
func (p *Plugin) Connected() bool {
	return p.connected.Load()
}

func (p *Plugin) OnClose(_ *gws.Conn, err error) {
	p.connected.Store(false)
	slog.InfoContext(p.Ctx, "deepgram_asr: websocket closed", "err", err)
}

func (p *Plugin) OnMessage(_ *gws.Conn, message *gws.Message) {
	body := message.Bytes()
	defer message.Close()

	var msg serverMessage
	if err := sonic.Unmarshal(body, &msg); err != nil {
		slog.ErrorContext(p.Ctx, "deepgram_asr: unmarshal server message failed",
			"raw", string(body), "error", err)
		return
	}

	switch msg.Type {
	case typeResults:
		p.handleResults(&msg)
	case typeSpeechStart:
		slog.DebugContext(p.Ctx, "deepgram_asr: speech started")
	case typeUtteranceEnd:
		slog.DebugContext(p.Ctx, "deepgram_asr: utterance end")
	case typeMetadata:
		// Metadata confirmation from Deepgram – nothing to do.
	case typeError:
		slog.ErrorContext(p.Ctx, "deepgram_asr: server error",
			"code", msg.ErrCode, "message", msg.ErrMsg)
		// Mark disconnected so the next audio frame triggers a reconnect.
		if p.connected.CompareAndSwap(true, false) {
			p.socket.WriteAsync(gws.OpcodeCloseConnection, nil, nil)
		}
	default:
		slog.DebugContext(p.Ctx, "deepgram_asr: unhandled message type", "type", msg.Type)
	}
}

// handleResults processes a Deepgram Results event and forwards the transcript
// to the workflow via BasePlugin.HandleTranscription.
func (p *Plugin) handleResults(msg *serverMessage) {
	if msg.Channel == nil || len(msg.Channel.Alternatives) == 0 {
		return
	}
	transcript := msg.Channel.Alternatives[0].Transcript
	if transcript == "" && !msg.IsFinal {
		return
	}

	slog.DebugContext(p.Ctx, "deepgram_asr: result",
		"transcript", transcript,
		"is_final", msg.IsFinal,
		"speech_final", msg.SpeechFinal)

	data := &asr.UserTranscription{
		Text:  transcript,
		Final: msg.IsFinal && msg.SpeechFinal,
	}
	p.HandleTranscription(data)
}

// buildURL constructs the Deepgram live-streaming WebSocket URL with all
// required query parameters.
func (p *Plugin) buildURL() (string, error) {
	base := p.cfg.BaseUrl
	if base == "" {
		base = "wss://api.deepgram.com/v1/listen"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()

	if p.cfg.Model != "" {
		q.Set("model", p.cfg.Model)
	} else {
		q.Set("model", "nova-3")
	}
	if p.cfg.Language != "" {
		q.Set("language", p.cfg.Language)
	}

	q.Set("interim_results", "true")

	q.Set("vad_events", "true")

	endpointMs := p.cfg.EndpointingMs
	if endpointMs <= 0 {
		endpointMs = 300
	}
	q.Set("endpointing", fmt.Sprintf("%d", endpointMs))

	// utterance_end_ms works together with interim_results and vad_events to
	// send an UtteranceEnd event after this many ms of silence.
	utteranceEndMs := p.cfg.UtteranceEndMs
	if utteranceEndMs <= 0 {
		utteranceEndMs = 1000
	}
	q.Set("utterance_end_ms", fmt.Sprintf("%d", utteranceEndMs))

	// Audio is raw 16-bit signed little-endian PCM at 16 kHz, mono.
	q.Set("encoding", "linear16")
	q.Set("sample_rate", "16000")
	q.Set("channels", "1")

	// Smart formatting gives us punctuation and capitalisation for free.
	q.Set("smart_format", "true")
	q.Set("punctuate", "true")

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "deepgram_asr",
		Description: "Deepgram real-time streaming ASR with server-side VAD",
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "role", engine.TypeString, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
