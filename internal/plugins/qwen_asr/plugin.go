package qwen_asr

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/asr"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	asr.BasePlugin
	gws.BuiltinEventHandler
	socket    *gws.Conn
	cfg       *QwenConfig
	connected atomic.Bool
}

func NewPlugin(configure *QwenConfig) engine.Plugin {
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
	header := http.Header{}
	header.Set("Authorization", "Bearer "+p.cfg.ApiKey)
	u, err := url.Parse(p.cfg.BaseUrl)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("model", p.cfg.Model)
	u.RawQuery = q.Encode()
	socket, resp, err := gws.NewClient(p, &gws.ClientOption{
		Addr:          u.String(),
		RequestHeader: header,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	p.socket = socket
	go socket.ReadLoop()
	p.connected.Store(true)
	return p.writeInitializationMessage()
}

func (p *Plugin) SendAudioData(data []byte, _ bool) error {
	if !p.connected.Load() {
		return errors.New("not connected")
	}
	audioBase64 := base64.StdEncoding.EncodeToString(data)
	message := &Message{
		EventID: uuid.New().String(),
		Type:    TypeAppend,
		Audio:   audioBase64,
	}
	payload, err := sonic.Marshal(message)
	if err != nil {
		return err
	}
	return p.socket.WriteMessage(gws.OpcodeText, payload)
}

func (p *Plugin) Stop() {
	if !p.connected.Load() {
		return
	}
	p.connected.Store(false)
	if p.socket != nil {
		p.socket.WriteAsync(gws.OpcodeCloseConnection, nil, nil)
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
	slog.InfoContext(p.Ctx, "websocket on close", "err", err)
}

func (p *Plugin) OnMessage(_ *gws.Conn, message *gws.Message) {
	var payload Message
	body := message.Bytes()
	defer message.Close()
	if err := sonic.Unmarshal(body, &payload); err != nil {
		slog.ErrorContext(p.Ctx, "Unmarshal server message failed", "message", string(body), "error", err)
		return
	}
	switch payload.Type {
	case TypeCreated:
		slog.InfoContext(p.Ctx, "ASR Session created", "session_id", payload.Session.ID)
	case TypeUpdated:
		slog.InfoContext(p.Ctx, "ASR Session updated", "session_id", payload.Session.ID)
	case TypeSpeechStart:
		slog.InfoContext(p.Ctx, "User Speech Started", "item_id", payload.ItemID,
			"audi_start_ms", payload.AudioStartMS)
	case TypeSpeechStopped:
		slog.InfoContext(p.Ctx, "User Speech Started", "item_id", payload.ItemID,
			"audi_end_ms", payload.AudioEndMS)
	case TypeItemCreated:
		slog.InfoContext(p.Ctx, "User item created", "event_id", payload.EventID)
	case TypeText:
		slog.InfoContext(p.Ctx, "Received text message", "text", payload.Text, "stash", payload.Stash)
		data := &asr.UserTranscription{
			Text:  payload.Stash,
			Final: false,
		}
		p.HandleTranscription(data)
	case TypeCompleted:
		slog.InfoContext(p.Ctx, "Received Completed message", "transcript", payload.Transcript)
		data := &asr.UserTranscription{
			Text:    payload.Transcript,
			Final:   true,
			Emotion: payload.Emotion,
		}
		p.HandleTranscription(data)
	case TypeFailed:
		slog.WarnContext(p.Ctx, "Received Failed message", "code", payload.Error.Code,
			"message", payload.Error.Message)
	case TypeError:
		slog.ErrorContext(p.Ctx, "Received Error message")
	}
}

func (p *Plugin) writeInitializationMessage() error {
	message := &Message{
		EventID: uuid.New().String(),
		Type:    TypeUpdate,
		Session: &MessageSession{
			InputAudioFormat: "pcm",
			SampleRate:       16000,
			TurnDetection: &TurnDetection{
				Type:              "server_vad",
				Threshold:         p.cfg.Threshold,
				SilenceDurationMs: p.cfg.SilenceDuration,
			},
		},
	}
	payload, _ := sonic.Marshal(message)
	return p.socket.WriteMessage(gws.OpcodeText, payload)
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name:        "qwen_asr",
		Description: "Qwen realtime asr",
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadASRResult, "text", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "is_final", engine.TypeBoolean, true).
			AddPayload(schema.PayloadASRResult, "role", engine.TypeString, true).
			AddPayload(schema.PayloadASRResult, "emotion", engine.TypeString, false).
			Build(),
	}); err != nil {
		panic(err)
	}
}
