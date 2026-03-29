package deepgram_asr

import (
	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

// Server-sent message types from Deepgram.
const (
	typeResults      = "Results"
	typeSpeechStart  = "SpeechStarted"
	typeUtteranceEnd = "UtteranceEnd"
	typeMetadata     = "Metadata"
	typeError        = "error"
)

//nolint:lll // struct tags are intentionally long for jsonschema
type DeepgramConfig struct {
	ApiKey         string `mapstructure:"api_key" json:"api_key" jsonschema:"title=API Key,description=Deepgram API Key (starts with your project key)"`
	BaseUrl        string `mapstructure:"base_url" json:"base_url" jsonschema:"title=Base URL,description=Deepgram WebSocket endpoint,default=wss://api.deepgram.com/v1/listen"`
	Model          string `mapstructure:"model" json:"model" jsonschema:"title=Model,description=Deepgram model name,default=nova-3"`
	Language       string `mapstructure:"language" json:"language" jsonschema:"title=Language,description=BCP-47 language tag (e.g. en-US zh-CN),default=zh-CN"`
	EndpointingMs  int    `mapstructure:"endpointing_ms" json:"endpointing_ms" jsonschema:"title=Endpointing (ms),description=Silence duration in ms before Deepgram finalizes a speech segment,default=300"`
	UtteranceEndMs int    `mapstructure:"utterance_end_ms" json:"utterance_end_ms" jsonschema:"title=Utterance End (ms),description=How long (ms) to wait after the last word before emitting UtteranceEnd,default=1000"`
}

func (d *DeepgramConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(d)
}

func (d *DeepgramConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, d)
}

// serverMessage is the top-level shape for all messages sent by Deepgram.
type serverMessage struct {
	Type        string   `json:"type"`
	IsFinal     bool     `json:"is_final"`
	SpeechFinal bool     `json:"speech_final"`
	Channel     *channel `json:"channel,omitempty"`
	ErrCode     string   `json:"err_code,omitempty"`
	ErrMsg      string   `json:"err_msg,omitempty"`
}

type channel struct {
	Alternatives []alternative `json:"alternatives"`
}

type alternative struct {
	Transcript string  `json:"transcript"`
	Confidence float64 `json:"confidence"`
}
