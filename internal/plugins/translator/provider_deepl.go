package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const (
	ProviderDeepL = "deepl"
)

type DeepLProvider struct {
	apiKey     string
	endpoint   string
	extraProps map[string]any
	client     *http.Client
}

type deeplResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
	Message string `json:"message,omitempty"`
}

func NewDeepLProvider(_ context.Context, properties json.RawMessage) (Provider, error) {
	var raw map[string]any
	if len(properties) > 0 {
		if err := sonic.Unmarshal(properties, &raw); err != nil {
			return nil, fmt.Errorf("deepl: invalid properties JSON: %w", err)
		}
	} else {
		raw = make(map[string]any)
	}

	apiKey, _ := raw["api_key"].(string)
	if apiKey == "" {
		return nil, errors.New("deepl: api_key is required in properties")
	}

	endpoint, _ := raw["endpoint"].(string)
	if endpoint == "" {
		if strings.HasSuffix(apiKey, ":fx") {
			endpoint = "https://api-free.deepl.com/v2/translate"
		} else {
			endpoint = "https://api.deepl.com/v2/translate"
		}
	}

	// Filter out api_key and endpoint to keep extra parameters for passthrough
	extraProps := make(map[string]any)
	for k, v := range raw {
		if k != "api_key" && k != "endpoint" {
			extraProps[k] = v
		}
	}

	// If extra_body was explicitly nested, merge its fields
	if extraBody, ok := raw["extra_body"].(map[string]any); ok {
		delete(extraProps, "extra_body")
		for k, v := range extraBody {
			extraProps[k] = v
		}
	}

	return &DeepLProvider{
		apiKey:     apiKey,
		endpoint:   endpoint,
		extraProps: extraProps,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (d *DeepLProvider) Name() string {
	return ProviderDeepL
}

func (d *DeepLProvider) Close() error {
	return nil
}

func (d *DeepLProvider) TranslateStream(
	ctx context.Context,
	text string,
	opts TranslateOptions,
	handler StreamHandler,
) error {
	if strings.TrimSpace(text) == "" {
		return handler("", "", true)
	}

	reqBody := make(map[string]any)
	// DeepL expects text as an array of strings
	reqBody["text"] = []string{text}

	if target := ToDeepLTargetLang(opts.TargetLang); target != "" {
		reqBody["target_lang"] = target
	}
	if source := ToDeepLSourceLang(opts.SourceLang); source != "" {
		reqBody["source_lang"] = source
	}

	// Pass through extra properties (e.g. formality, glossary_id, split_sentences)
	for k, v := range d.extraProps {
		reqBody[k] = v
	}

	bodyBytes, err := sonic.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("deepl: marshal request error: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("deepl: new request error: %w", err)
	}

	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+d.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("deepl: http post error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("deepl: read response body error: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("deepl: api error status %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed deeplResponse
	if err := sonic.Unmarshal(respBytes, &parsed); err != nil {
		return fmt.Errorf("deepl: unmarshal response error: %w", err)
	}

	if len(parsed.Translations) == 0 {
		return errors.New("deepl: empty translations returned")
	}

	res := parsed.Translations[0].Text
	return handler(res, res, true)
}

func init() {
	RegisterProvider(ProviderDeepL, NewDeepLProvider)
}
