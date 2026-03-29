package openai_llm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/llm"
	"github.com/wnnce/voce/internal/schema"
)

type Plugin struct {
	llm.BasePlugin
	cfg    OpenaiConfig
	client *http.Client
}

func NewPlugin(cfg *OpenaiConfig) engine.Plugin {
	client := cfg.client
	if client == nil {
		client = http.DefaultClient
	}
	plg := &Plugin{
		cfg:    *cfg,
		client: client,
	}
	plg.Provider = plg
	return engine.NewMultiTrackPlugin(plg, engine.WithPayloadTrack(
		128, engine.BlockIfFull, schema.SignalInterrupter,
	))
}

func (p *Plugin) OnStart(ctx context.Context, flow engine.Flow) error {
	p.Init(ctx, flow, p.cfg.BaseConfig)
	return nil
}

func (p *Plugin) StreamChat(ctx context.Context, payload llm.ChatPayload, handler llm.StreamHandler) error {
	chatRequest := &ChatRequest{
		Model:    p.cfg.Model,
		Stream:   true,
		Messages: payload.Messages,
	}
	request, err := p.createRequest(ctx, chatRequest)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("request openai failed, status: %d, error: %s", resp.StatusCode, string(b))
	}
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		sline := strings.TrimSpace(string(line))
		if sline == "" || strings.HasPrefix(sline, ":") {
			continue
		}
		if !strings.HasPrefix(sline, "data:") {
			continue
		}
		dataPart := strings.TrimSpace(strings.TrimPrefix(sline, "data:"))
		if dataPart == "[DONE]" {
			break
		}
		var chunk ResultStreamChunk
		if err = sonic.Unmarshal([]byte(dataPart), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		streamChunk := &llm.StreamChunk{
			Content:          delta.Content,
			ReasoningContent: delta.ReasoningContent,
			FinishReason:     chunk.Choices[0].FinishReason,
		}
		handler(ctx, streamChunk)
	}
	return nil
}

func (p *Plugin) createRequest(ctx context.Context, request *ChatRequest) (*http.Request, error) {
	payload, err := sonic.Marshal(request)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(p.cfg.BaseUrl, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.ApiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.ApiKey)
	}
	return httpReq, nil
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name: "openai_llm",
		Inputs: engine.NewPropertyBuilder().
			AddWildPayload("text", engine.TypeString, true).
			AddWildPayload("is_final", engine.TypeBoolean, true).
			Build(),
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
