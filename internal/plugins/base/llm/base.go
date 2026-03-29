package llm

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/plugins/base/llm/data"
	"github.com/wnnce/voce/internal/schema"
)

const (
	minSentenceLen = 20
)

//nolint:lll // struct tags are intentionally long for jsonschema
type BaseConfig struct {
	Prompt        string `json:"prompt" jsonschema:"description=System prompt to guide the AI personality,default=You are a helpful assistant."`
	HistoryLimit  int    `json:"history_limit" jsonschema:"description=Maximum number of past turns to remember,default=10"`
	FailedMessage string `json:"failed_message" jsonschema:"description=Message to show when the AI fails to respond,default=I apologize, I am having trouble connecting right now."`
}

type ChatPayload struct {
	Messages []data.Message
	Params   map[string]any
}

type StreamChunk struct {
	FinishReason     string
	Content          string
	ReasoningContent string
}

type StreamHandler func(ctx context.Context, chunk *StreamChunk)

type Provider interface {
	StreamChat(ctx context.Context, payload ChatPayload, handler StreamHandler) error
}

type BasePlugin struct {
	engine.BuiltinPlugin
	Provider Provider
	Session  *CacheSession
	Config   BaseConfig
	Flow     engine.Flow
}

func (e *BasePlugin) Init(ctx context.Context, flow engine.Flow, config BaseConfig) {
	e.Config = config
	e.Flow = flow

	if e.Config.HistoryLimit <= 0 {
		e.Config.HistoryLimit = 10
	}
	e.Session = NewCacheSession("session", e.Config.HistoryLimit)
}

func (e *BasePlugin) OnPayload(ctx context.Context, _ engine.Flow, d schema.Payload) {
	isFinal := schema.GetAs(d, "is_final", false)
	text := schema.GetAs(d, "text", "")
	slog.InfoContext(ctx, "llm extension received data", "text", text, "is_final", isFinal)
	if !isFinal || text == "" {
		return
	}
	e.processMessage(ctx, text)
}

func (e *BasePlugin) processMessage(ctx context.Context, text string) {
	slog.InfoContext(ctx, "llm extension process message", "text", text)
	fragment := &strings.Builder{}
	userMessage := data.NewUserMessage("user", data.NewTextContent(text))

	err := e.streamChat(ctx, func(current context.Context, chunk string) {
		sentences := e.parseSentences(fragment, chunk)
		for _, s := range sentences {
			e.sendOutput(s, false)
		}
	}, userMessage)

	if err != nil && !errors.Is(err, context.Canceled) && e.Config.FailedMessage != "" {
		e.sendOutput(e.Config.FailedMessage, true)
		return
	}
	// Send any remaining fragment
	finalText := fragment.String()
	if strings.TrimSpace(finalText) != "" {
		e.sendOutput(finalText, true)
	} else {
		e.sendOutput("", true)
	}
}

func (e *BasePlugin) sendOutput(text string, isFinal bool) {
	out := schema.NewPayload(schema.PayloadLLMChunk)
	_ = out.Set("sentence", text)
	_ = out.Set("is_final", isFinal)
	e.Flow.SendPayload(out.ReadOnly())
}

func (e *BasePlugin) streamChat(ctx context.Context, handler func(context.Context, string), messages ...data.Message) error {
	history := e.Session.Messages()
	allMessages := append(history, messages...)

	if len(allMessages) > 0 {
		if _, ok := allMessages[0].(*data.SystemMessage); !ok && e.Config.Prompt != "" {
			allMessages = append([]data.Message{data.NewSystemMessage(e.Config.Prompt)}, allMessages...)
		}
	}

	var builder strings.Builder
	payload := ChatPayload{
		Messages: allMessages,
	}

	err := e.Provider.StreamChat(ctx, payload, func(ctx context.Context, chunk *StreamChunk) {
		if chunk.Content != "" {
			builder.WriteString(chunk.Content)
			handler(ctx, chunk.Content)
		}
	})

	if err != nil {
		return err
	}

	aiMsg := data.NewAiMessage(builder.String())
	e.Session.AddMessage(ctx, append(messages, aiMsg)...)
	return nil
}

func (e *BasePlugin) parseSentences(fragment *strings.Builder, content string) []string {
	sentences := make([]string, 0)
	for _, char := range content {
		fragment.WriteRune(char)

		if e.isBoundary(char) {
			str := fragment.String()
			if e.shouldSplit(str, char) {
				sentences = append(sentences, str)
				fragment.Reset()
			}
		}
	}
	return sentences
}

func (e *BasePlugin) isBoundary(r rune) bool {
	return r == '。' || r == '！' || r == '？' || r == '!' || r == '?' ||
		r == '，' || r == ',' || r == '；' || r == ';' || r == '：' || r == ':' ||
		r == '\n' || r == '\r'
}

func (e *BasePlugin) shouldSplit(s string, lastRune rune) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || !e.hasAlphaNum(trimmed) {
		return false
	}

	if lastRune == '。' || lastRune == '！' || lastRune == '？' ||
		lastRune == '!' || lastRune == '?' || lastRune == '\n' || lastRune == '\r' {
		return true
	}

	return len([]rune(trimmed)) >= minSentenceLen
}

func (e *BasePlugin) hasAlphaNum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
