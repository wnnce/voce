package openai_llm

import (
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/plugins/base/llm"
	"github.com/wnnce/voce/internal/plugins/base/llm/data"
)

type OpenaiConfig struct {
	llm.BaseConfig
	BaseUrl string `json:"base_url" jsonschema:"description=OpenAI API base URL,default=https://api.openai.com/v1"`
	ApiKey  string `json:"api_key" jsonschema:"description=Your OpenAI API key"`
	Model   string `json:"model" jsonschema:"description=Model identifier,default=gpt-4o"`
	client  *http.Client
}

func (o *OpenaiConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(o)
}

func (o *OpenaiConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, o)
}

// ChatRequest mirrors the OpenAI chat.completions request (stream: true)
type ChatRequest struct {
	Model     string         `json:"model"`
	Messages  []data.Message `json:"messages"`
	Stream    bool           `json:"stream"`
	ExtraBody map[string]any `json:"extra_body,omitempty"`
}

// ResultStreamChunk minimal fields used from SSE data: { id, choices: [{ delta: { content } }] }
type ResultStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}
