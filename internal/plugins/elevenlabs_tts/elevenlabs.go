package elevenlabs_tts

import (
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

//nolint:lll // struct tags are intentionally long for jsonschema
type ElevenLabsConfig struct {
	ApiKey          string  `mapstructure:"api_key" json:"api_key" jsonschema:"title=API Key,description=ElevenLabs API key (xi-api-key)"`
	BaseURL         string  `mapstructure:"base_url" json:"base_url" jsonschema:"title=Base URL,description=ElevenLabs API base URL,default=https://api.elevenlabs.io"`
	VoiceID         string  `mapstructure:"voice_id" json:"voice_id" jsonschema:"title=Voice ID,description=ElevenLabs voice ID"`
	ModelID         string  `mapstructure:"model_id" json:"model_id" jsonschema:"title=Model ID,description=ElevenLabs model ID,default=eleven_turbo_v2_5"`
	Stability       float64 `mapstructure:"stability" json:"stability" jsonschema:"title=Stability,description=Voice stability (0-1),default=0.5,minimum=0,maximum=1"`
	SimilarityBoost float64 `mapstructure:"similarity_boost" json:"similarity_boost" jsonschema:"title=Similarity Boost,description=Voice similarity boost (0-1),default=0.75,minimum=0,maximum=1"`
	client          *http.Client
}

func (c *ElevenLabsConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(c)
}

func (c *ElevenLabsConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, c)
}

// streamRequest is the JSON body sent to POST /v1/text-to-speech/{voice_id}/stream.
type streamRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id"`
	PreviousText  string         `json:"previous_text,omitempty"`
	OutputFormat  string         `json:"output_format"`
	VoiceSettings *voiceSettings `json:"voice_settings,omitempty"`
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}
