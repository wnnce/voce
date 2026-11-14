package translator

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/invopop/jsonschema"
)

type ProviderConfig struct {
	Type       string          `json:"type" jsonschema:"title=Provider Type,description=Provider type: deepl, openai, volcengine, or mock,default=deepl"`            //nolint:lll // struct tag exceeds line length
	Properties json.RawMessage `json:"properties,omitempty" jsonschema:"title=Provider Properties,description=Provider-specific configuration and extra parameters"` //nolint:lll // struct tag exceeds line length
}

type TranslationConfig struct {
	SourceLang     string          `json:"source_lang,omitempty" jsonschema:"title=Source Language,description=Source language BCP 47 code (e.g. en-US or auto),default=auto"`                                             //nolint:lll // struct tag exceeds line length
	TargetLang     string          `json:"target_lang" jsonschema:"title=Target Language,description=Target language BCP 47 code (e.g. zh-CN),default=zh-CN"`                                                              //nolint:lll // struct tag exceeds line length
	DebounceMs     int             `json:"debounce_ms,omitempty" jsonschema:"title=Debounce MS,description=Debounce window in ms for intermediate draft translations,default=250"`                                         //nolint:lll // struct tag exceeds line length
	SplitSentences bool            `json:"split_sentences,omitempty" jsonschema:"title=Split Sentences,description=Whether to commit on strong punctuation like periods,default=false"`                                    //nolint:lll // struct tag exceeds line length
	FastProvider   ProviderConfig  `json:"fast_provider" jsonschema:"title=Fast Translation Provider,description=Provider configuration for fast interim draft translations"`                                              //nolint:lll // struct tag exceeds line length
	FinalProvider  *ProviderConfig `json:"final_provider,omitempty" jsonschema:"title=Final Translation Provider,description=Optional provider for final committed translations (falls back to fast_provider if omitted)"` //nolint:lll // struct tag exceeds line length
}

func (c *TranslationConfig) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(c)
}

func (c *TranslationConfig) Decode(data []byte) error {
	return sonic.Unmarshal(data, c)
}
