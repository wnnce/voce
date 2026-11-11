package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// StreamHandler receives streaming delta tokens and the accumulated translation so far.
// isDone indicates whether this is the final chunk of the stream.
type StreamHandler func(delta string, accumulated string, isDone bool) error

// TranslateOptions encapsulates contextual options for a translation request.
type TranslateOptions struct {
	SourceLang string // BCP 47 code (e.g. "en-US", "auto")
	TargetLang string // BCP 47 code (e.g. "zh-CN")
	IsFinal    bool   // Whether this is the final committed translation of an utterance
	DraftRef   string // Optional reference draft previously shown to the user (for consistency)
}

// Provider defines the interface for translation backends (MT or LLM).
type Provider interface {
	TranslateStream(ctx context.Context, text string, opts TranslateOptions, handler StreamHandler) error
	Name() string
	Close() error
}

type ProviderFactory func(ctx context.Context, properties json.RawMessage) (Provider, error)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]ProviderFactory)
)

// RegisterProvider registers a provider constructor by name.
func RegisterProvider(name string, factory ProviderFactory) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = factory
}

// NewProvider instantiates a translation provider based on ProviderConfig.
func NewProvider(ctx context.Context, cfg ProviderConfig) (Provider, error) {
	providersMu.RLock()
	factory, ok := providers[cfg.Type]
	providersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("translator: unsupported provider %q", cfg.Type)
	}
	return factory(ctx, cfg.Properties)
}
