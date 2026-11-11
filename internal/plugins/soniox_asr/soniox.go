package soniox_asr

import (
	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

//nolint:lll // struct tags are intentionally long for jsonschema
type SonioxConfig struct {
	ApiKey                   string `mapstructure:"api_key" json:"api_key" jsonschema:"title=API Key,description=Soniox API Key"`
	BaseUrl                  string `mapstructure:"base_url" json:"base_url" jsonschema:"title=Base URL,description=Soniox WebSocket endpoint,default=wss://stt-rt.soniox.com/transcribe-websocket"`
	Model                    string `mapstructure:"model" json:"model" jsonschema:"title=Model,description=Soniox model name,default=stt-rt-v6"`
	LanguageHints            string `mapstructure:"language_hints" json:"language_hints" jsonschema:"title=Language Hints,description=Comma-separated language codes (e.g. zh,en),default=zh"`
	EnableSpeakerDiarization bool   `mapstructure:"enable_speaker_diarization" json:"enable_speaker_diarization" jsonschema:"title=Enable Diarization,description=Enable multi-speaker diarization,default=false"`
	EnableEndpointDetection  bool   `mapstructure:"enable_endpoint_detection" json:"enable_endpoint_detection" jsonschema:"title=Enable Endpoint Detection,description=Enable semantic endpoint detection,default=true"`
}

func (s *SonioxConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(s)
}

func (s *SonioxConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, s)
}

// clientConfig is sent as the initial text frame upon opening the WebSocket.
type clientConfig struct {
	Model                    string   `json:"model"`
	AudioFormat              string   `json:"audio_format"`
	SampleRate               int      `json:"sample_rate"`
	NumChannels              int      `json:"num_channels"`
	LanguageHints            []string `json:"language_hints,omitempty"`
	EnableSpeakerDiarization bool     `json:"enable_speaker_diarization"`
	EnableEndpointDetection  bool     `json:"enable_endpoint_detection"`
}

// serverResponse is the JSON payload received from Soniox WebSocket.
type serverResponse struct {
	Tokens           []token `json:"tokens"`
	TotalAudioProcMs int     `json:"total_audio_proc_ms,omitempty"`
	FinalAudioProcMs int     `json:"final_audio_proc_ms,omitempty"`
	ErrorCode        int     `json:"error_code,omitempty"`
	ErrorMessage     string  `json:"error_message,omitempty"`
}

type token struct {
	Text    string `json:"text"`
	IsFinal bool   `json:"is_final"`
	Speaker string `json:"speaker,omitempty"`
	StartMs int    `json:"start_ms,omitempty"`
	EndMs   int    `json:"end_ms,omitempty"`
}
