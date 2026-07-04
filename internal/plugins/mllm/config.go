package mllm

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

//nolint:lll // struct tags are intentionally long for jsonschema
type RealtimeConfig struct {
	Provider         string          `json:"provider" jsonschema:"title=Provider,description=Realtime provider name. Supported values: qwen or openai,default=qwen"`
	InputSampleRate  int             `json:"input_sample_rate" jsonschema:"title=Input Sample Rate,description=Input audio sample rate expected by the realtime provider,default=16000"`
	OutputSampleRate int             `json:"output_sample_rate" jsonschema:"title=Output Sample Rate,description=Output audio sample rate produced by the realtime provider,default=16000"`
	OutputChannels   int             `json:"output_channels" jsonschema:"title=Output Channels,description=Number of output audio channels produced by the realtime provider,default=1"`
	Properties       json.RawMessage `json:"properties" jsonschema:"title=Provider Properties,description=Provider-specific realtime configuration as a JSON object"`
}

func (r *RealtimeConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(r)
}

func (r *RealtimeConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, r)
}
