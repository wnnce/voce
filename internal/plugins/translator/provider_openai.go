package translator

import (
	"bufio"
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
	ProviderOpenAI = "openai"
)

type OpenAIProvider struct {
	baseUrl      string
	apiKey       string
	model        string
	systemPrompt string
	extraProps   map[string]any
	client       *http.Client
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func NewOpenAIProvider(_ context.Context, properties json.RawMessage) (Provider, error) {
	var raw map[string]any
	if len(properties) > 0 {
		if err := sonic.Unmarshal(properties, &raw); err != nil {
			return nil, fmt.Errorf("openai: invalid properties JSON: %w", err)
		}
	} else {
		raw = make(map[string]any)
	}

	baseUrl, _ := raw["base_url"].(string)
	if baseUrl == "" {
		baseUrl = "https://api.openai.com/v1"
	}
	baseUrl = strings.TrimRight(baseUrl, "/")

	apiKey, _ := raw["api_key"].(string)

	model, _ := raw["model"].(string)
	if model == "" {
		model = "gpt-4o-mini"
	}

	systemPrompt, _ := raw["system_prompt"].(string)
	if systemPrompt == "" {
		systemPrompt = "You are a professional real-time simultaneous interpreter. " +
			"Translate the user text accurately and naturally. " +
			"Output ONLY the translated text, without explanations or quotation marks."
	}

	// Filter out base_url, api_key, model, system_prompt to keep extra parameters for passthrough
	extraProps := make(map[string]any)
	for k, v := range raw {
		if k != "base_url" && k != "api_key" && k != "model" && k != "system_prompt" {
			extraProps[k] = v
		}
	}

	// If extra_body was explicitly specified as a nested map, merge its fields
	if extraBody, ok := raw["extra_body"].(map[string]any); ok {
		delete(extraProps, "extra_body")
		for k, v := range extraBody {
			extraProps[k] = v
		}
	}

	return &OpenAIProvider{
		baseUrl:      baseUrl,
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		extraProps:   extraProps,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}

func (o *OpenAIProvider) Name() string {
	return ProviderOpenAI
}

func (o *OpenAIProvider) Close() error {
	return nil
}

func (o *OpenAIProvider) buildUserPrompt(opts TranslateOptions, text string) string {
	targetLangDesc := ToOpenAILangDesc(opts.TargetLang)
	hasSource := opts.SourceLang != "" && !strings.EqualFold(opts.SourceLang, "auto")

	if opts.IsFinal && opts.DraftRef != "" {
		langClause := fmt.Sprintf("into %s", targetLangDesc)
		if hasSource {
			langClause = fmt.Sprintf("from %s into %s", ToOpenAILangDesc(opts.SourceLang), targetLangDesc)
		}
		return fmt.Sprintf(
			"Translate the following speech %s:\n\n%s\n\n"+
				"Reference draft translation previously displayed to the audience:\n%s\n\n"+
				"Instructions:\n"+
				"1. Provide the final, complete translation in %s.\n"+
				"2. Maintain terminology and phrasing consistency with the reference draft if it is accurate, "+
				"while ensuring the entire sentence is grammatically fluent and idiomatic.\n"+
				"3. Output ONLY the translated text, without explanations or quotation marks.",
			langClause, text, opts.DraftRef, targetLangDesc,
		)
	}

	if hasSource {
		return fmt.Sprintf(
			"Translate the following text from %s into %s. Output ONLY the translated text:\n\n%s",
			ToOpenAILangDesc(opts.SourceLang), targetLangDesc, text,
		)
	}
	return fmt.Sprintf("Translate the following text into %s. Output ONLY the translated text:\n\n%s", targetLangDesc, text)
}

func parseSSELine(line []byte) (string, bool, error) {
	sline := strings.TrimSpace(string(line))
	if sline == "" || strings.HasPrefix(sline, ":") || !strings.HasPrefix(sline, "data:") {
		return "", false, nil
	}
	dataPart := strings.TrimSpace(strings.TrimPrefix(sline, "data:"))
	if dataPart == "[DONE]" {
		return "", true, nil
	}
	var chunk openAIStreamChunk
	if err := sonic.Unmarshal([]byte(dataPart), &chunk); err != nil {
		return "", false, nil
	}
	if chunk.Error != nil && chunk.Error.Message != "" {
		return "", false, fmt.Errorf("openai stream error: %s", chunk.Error.Message)
	}
	if len(chunk.Choices) == 0 {
		return "", false, nil
	}
	return chunk.Choices[0].Delta.Content, false, nil
}

func (o *OpenAIProvider) TranslateStream(
	ctx context.Context,
	text string,
	opts TranslateOptions,
	handler StreamHandler,
) error {
	if strings.TrimSpace(text) == "" {
		return handler("", "", true)
	}

	userPrompt := o.buildUserPrompt(opts, text)

	reqBody := make(map[string]any)
	reqBody["model"] = o.model
	reqBody["stream"] = true
	reqBody["messages"] = []openAIChatMessage{
		{Role: "system", Content: o.systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	for k, v := range o.extraProps {
		reqBody[k] = v
	}

	bodyBytes, err := sonic.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("openai: marshal request error: %w", err)
	}

	url := o.baseUrl + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("openai: new request error: %w", err)
	}

	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("openai: http post error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("openai: api error status %d: %s", resp.StatusCode, string(respBytes))
	}

	reader := bufio.NewReader(resp.Body)
	var accumulated strings.Builder

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		delta, isDone, parseErr := parseSSELine(line)
		if parseErr != nil {
			return parseErr
		}
		if isDone {
			break
		}
		if delta != "" {
			accumulated.WriteString(delta)
			if err := handler(delta, accumulated.String(), false); err != nil {
				return err
			}
		}
	}

	return handler("", accumulated.String(), true)
}

func init() {
	RegisterProvider(ProviderOpenAI, NewOpenAIProvider)
}
