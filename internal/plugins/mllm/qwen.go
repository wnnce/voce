package mllm

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/lxzan/gws"
)

var (
	errQwenRealtimeNotConnected    = errors.New("qwen realtime: not connected")
	errQwenRealtimeTextUnsupported = errors.New("qwen realtime: text input is not supported")
)

const (
	stateIdle = iota
	stateListen
	stateUserSpeaking
	stateAgentSpeaking
	stateWaiting
	stateToolCalling
	stateError
)

type QwenConfig struct {
	Url     string `json:"url"`
	ApiKey  string `json:"api_key"`
	Model   string `json:"model"`
	Session QwenRealtimeSessionOptions
}

type QwenRealtimeProvider struct {
	gws.BuiltinEventHandler
	ctx       context.Context
	socket    *gws.Conn
	connected atomic.Bool
	state     atomic.Int64
	listener  RealtimeListener
	itemId    string
	cfg       QwenConfig
}

func NewQwenRealtimeProvider(ctx context.Context, cfg QwenConfig) (RealtimeProvider, error) {
	if cfg.Url == "" {
		return nil, errors.New("qwen realtime: url is required")
	}
	if cfg.ApiKey == "" {
		return nil, errors.New("qwen realtime: api_key is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("qwen realtime: model is required")
	}
	return &QwenRealtimeProvider{
		ctx: ctx,
		cfg: cfg,
	}, nil
}

func (q *QwenRealtimeProvider) Connect(ctx context.Context) error {
	if q.connected.Load() {
		return nil
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+q.cfg.ApiKey)
	u, err := url.Parse(q.cfg.Url)
	if err != nil {
		return err
	}
	query := u.Query()
	query.Set("model", q.cfg.Model)
	u.RawQuery = query.Encode()
	socket, resp, err := gws.NewClient(q, &gws.ClientOption{
		Addr:          u.String(),
		RequestHeader: header,
	})
	if err != nil {
		return err
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	go socket.ReadLoop()
	return nil
}

func (q *QwenRealtimeProvider) Listen(listener RealtimeListener) {
	q.listener = listener
}

func (q *QwenRealtimeProvider) Name() string {
	return ProviderQwen
}

func (q *QwenRealtimeProvider) SendAudio(data []byte) error {
	message := NewQwenRealtimeInputAudioBufferAppendEvent("", base64.StdEncoding.EncodeToString(data))
	return q.send(message)
}

func (q *QwenRealtimeProvider) SendText(text string) error {
	return errQwenRealtimeTextUnsupported
}

func (q *QwenRealtimeProvider) SendImage(data []byte) error {
	message := NewQwenRealtimeInputImageBufferAppendEvent("", base64.StdEncoding.EncodeToString(data))
	return q.send(message)
}

func (q *QwenRealtimeProvider) SendToolCallAnswer(answer ToolCallAnswer) error {
	state := q.state.Load()
	if state != stateToolCalling {
		return fmt.Errorf("qwen realtime: cannot send tool call answer while state is %d", state)
	}
	message := NewQwenRealtimeConversationItemCreateEvent("", QwenRealtimeConversationItemOutput{
		ID:     q.itemId,
		Type:   "",
		CallID: answer.CallID,
		Output: answer.Result,
	})
	if err := q.send(message); err != nil {
		return err
	}
	create := NewQwenRealtimeResponseCreateEvent("")
	return q.send(create)
}

func (q *QwenRealtimeProvider) Interrupter() error {
	clean := NewQwenRealtimeInputAudioBufferClearEvent("")
	if err := q.send(clean); err != nil {
		return err
	}
	switch q.state.Load() {
	case stateAgentSpeaking, stateWaiting, stateToolCalling:
		cancel := NewQwenRealtimeResponseCancelEvent("")
		return q.send(cancel)
	default:
	}
	return nil
}

func (q *QwenRealtimeProvider) Connected() bool {
	return q.connected.Load()
}

func (q *QwenRealtimeProvider) Close() {
	if !q.connected.Load() || q.socket == nil {
		return
	}
	_ = q.socket.WriteClose(1000, nil)
}

func (q *QwenRealtimeProvider) send(message QwenRealtimeClientMessage) error {
	if !q.connected.Load() || q.socket == nil {
		return errQwenRealtimeNotConnected
	}
	payload, err := sonic.Marshal(message)
	if err != nil {
		return err
	}
	return q.socket.WriteMessage(gws.OpcodeText, payload)
}
