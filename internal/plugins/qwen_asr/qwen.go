package qwen_asr

import (
	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

const (
	TypeUpdate        = "session.update"
	TypeAppend        = "input_audio_buffer.append"
	TypeError         = "error"
	TypeCreated       = "session.created"
	TypeUpdated       = "session.updated"
	TypeSpeechStart   = "input_audio_buffer.speech_started"
	TypeSpeechStopped = "input_audio_buffer.speech_stopped"
	TypeCommitted     = "input_audio_buffer.committed"
	TypeItemCreated   = "conversation.item.created"
	TypeText          = "conversation.item.input_audio_transcription.text"
	TypeCompleted     = "conversation.item.input_audio_transcription.completed"
	TypeFailed        = "conversation.item.input_audio_transcription.failed"
)

//nolint:lll // struct tags are intentionally long for jsonschema
type QwenConfig struct {
	BaseUrl         string  `mapstructure:"base_url" json:"base_url" jsonschema:"title=Base URL,description=The WebSocket URL for Qwen Realtime ASR API,default=wss://dashscope.aliyuncs.com/api-ilsh/v1/ai-agent/realtime"`
	ApiKey          string  `mapstructure:"api_key" json:"api_key" jsonschema:"title=API Key,description=DashScope API Key (sk-xxx)"`
	Model           string  `mapstructure:"model" json:"model" jsonschema:"title=Model,description=Qwen model name,default=qwen-realtime-v1"`
	Threshold       float64 `mapstructure:"threshold" json:"threshold" jsonschema:"title=VAD Threshold,description=The threshold for Voice Activity Detection (0.0 to 1.0),default=0.5"`
	SilenceDuration int     `mapstructure:"silence_duration" json:"silence_duration" jsonschema:"title=Silence Duration,description=Duration of silence in milliseconds to trigger turn end (ms),default=500"`
}

func (q *QwenConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(q)
}

func (q *QwenConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, q)
}

type Message struct {
	EventID        string          `json:"event_id"`
	Type           string          `json:"type"`
	Session        *MessageSession `json:"session,omitempty"`
	Audio          string          `json:"audio,omitempty"`
	Error          *MessageError   `json:"error,omitempty"`
	AudioStartMS   int             `json:"audio_start_ms,omitempty"`
	AudioEndMS     int             `json:"audio_end_ms,omitempty"`
	PreviousItemId string          `json:"previous_item_id,omitempty"`
	ItemID         string          `json:"item_id,omitempty"`
	ContentIndex   int             `json:"content_index,omitempty"`
	Language       string          `json:"language,omitempty"`
	Emotion        string          `json:"emotion,omitempty"`
	Text           string          `json:"text,omitempty"`
	Stash          string          `json:"stash,omitempty"`
	Transcript     string          `json:"transcript,omitempty"`
}

type MessageSession struct {
	ID                      string                   `json:"id,omitempty"`
	Object                  string                   `json:"object,omitempty"`
	Model                   string                   `json:"model,omitempty"`
	Modalities              []string                 `json:"modalities,omitempty"`
	InputAudioFormat        string                   `json:"input_audio_format,omitempty"`
	SampleRate              int                      `json:"sample_rate,omitempty"`
	InputAudioTranscription *InputAudioTranscription `json:"input_audio_transcription,omitempty"`
	TurnDetection           *TurnDetection           `json:"turn_detection,omitempty"`
}

type MessageError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param"`
	EventID string `json:"event_id"`
}

type InputAudioTranscription struct {
	Language string `json:"language,omitempty"`
	Corpus   Corpus `json:"corpus"`
}

type Corpus struct {
	Text string `json:"text"`
}

type TurnDetection struct {
	Type              string  `json:"type"`
	Threshold         float64 `json:"threshold"`
	SilenceDurationMs int     `json:"silence_duration_ms"`
}
