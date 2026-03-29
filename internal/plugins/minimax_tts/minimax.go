package minimax_tts

import (
	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

type VoiceSetting struct {
	VoiceId string  `json:"voice_id"`
	Speed   float32 `json:"speed,omitempty"`
	Vol     int     `json:"vol,omitempty"`
	Pitch   int     `json:"pitch,omitempty"`
	Emotion string  `json:"emotion,omitempty"`
}

type AudioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format"`
	Channel    int    `json:"channel,omitempty"`
}

type MinimaxOptions struct {
	Model         string         `json:"model"`
	Stream        bool           `json:"stream,omitempty"`
	VoiceSetting  *VoiceSetting  `json:"voice_setting,omitempty"`
	AudioSetting  *AudioSetting  `json:"audio_setting,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	LanguageBoost string         `json:"language_boost,omitempty"`
}

type StreamOptions struct {
	ExcludeAggregatedAudio bool `json:"exclude_aggregated_audio"`
}

type MinimaxRequest struct {
	MinimaxOptions
	Event string `json:"event,omitempty"`
	Text  string `json:"text,omitempty"`
}

type MinimaxResponse struct {
	Data      *MinimaxData      `json:"data,omitempty"`
	Event     string            `json:"event,omitempty"`
	ExtraInfo *MinimaxExtraInfo `json:"extra_info,omitempty"`
	IsFinal   bool              `json:"is_final"`
	SessionID string            `json:"session_id"`
	TraceID   string            `json:"trace_id"`
	BaseResp  MinimaxBaseResp   `json:"base_resp"`
}

type MinimaxData struct {
	Audio string `json:"audio"`
}

type MinimaxExtraInfo struct {
	AudioChannel            int     `json:"audio_channel"`
	AudioFormat             string  `json:"audio_format"`
	AudioLength             int     `json:"audio_length"`
	AudioSampleRate         int     `json:"audio_sample_rate"`
	AudioSize               int     `json:"audio_size"`
	Bitrate                 int     `json:"bitrate"`
	InvisibleCharacterRatio float64 `json:"invisible_character_ratio"`
	UsageCharacters         int     `json:"usage_characters"`
	WordCount               int     `json:"word_count"`
}

type MinimaxBaseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

//nolint:lll // struct tags are intentionally long for jsonschema
type MinimaxConfig struct {
	BaseUrl string `mapstructure:"base_url" json:"base_url" jsonschema:"description=Minimax API base URL,default=https://api.minimax.chat/v1/t2a_v2"`
	Model   string `mapstructure:"model" json:"model" jsonschema:"description=Model identifier,default=speech-01-turbo"`
	Token   string `mapstructure:"token" json:"token" jsonschema:"description=Your Minimax API token"`
	VoiceID string `mapstructure:"voice_id" json:"voice_id" jsonschema:"description=Voice identifier,default=male-qn-qingse"`
}

func (m *MinimaxConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(m)
}

func (m *MinimaxConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, m)
}
