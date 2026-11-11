package translator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const (
	ProviderMock = "mock"
)

type MockProvider struct {
	Prefix        string
	Rules         map[string]string
	DelayMs       int
	TranslateHook func(ctx context.Context, text string, opts TranslateOptions, handler StreamHandler) error
}

func NewMockProvider(_ context.Context, properties json.RawMessage) (Provider, error) {
	p := &MockProvider{
		Prefix: "Trans: ",
		Rules:  make(map[string]string),
	}
	if len(properties) > 0 {
		var raw struct {
			Prefix  string            `json:"prefix"`
			Rules   map[string]string `json:"rules"`
			DelayMs int               `json:"delay_ms"`
		}
		if err := sonic.Unmarshal(properties, &raw); err == nil {
			if raw.Prefix != "" {
				p.Prefix = raw.Prefix
			}
			if raw.Rules != nil {
				p.Rules = raw.Rules
			}
			p.DelayMs = raw.DelayMs
		}
	}
	return p, nil
}

func (m *MockProvider) Name() string {
	return ProviderMock
}

func (m *MockProvider) Close() error {
	return nil
}

func (m *MockProvider) TranslateStream(ctx context.Context, text string, opts TranslateOptions, handler StreamHandler) error {
	if m.TranslateHook != nil {
		return m.TranslateHook(ctx, text, opts, handler)
	}

	if m.DelayMs > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(m.DelayMs) * time.Millisecond):
		}
	}

	trimmed := strings.TrimSpace(text)
	val, ok := m.Rules[trimmed]
	if !ok {
		val = m.Prefix + trimmed
	}
	return handler(val, val, true)
}

func init() {
	RegisterProvider(ProviderMock, NewMockProvider)
}
