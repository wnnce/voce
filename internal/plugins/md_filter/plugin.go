package md_filter

import (
	"context"
	"strings"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

const (
	stateNormal int = iota
	stateInCodeBlock
	stateInLinkUrl
)

type Plugin struct {
	engine.BuiltinPlugin
	state            int
	backtickBuf      strings.Builder
	seenCloseBracket bool
}

func NewPlugin(_ engine.EmptyPluginConfig) engine.Plugin {
	return &Plugin{}
}

func (p *Plugin) OnSignal(ctx context.Context, flow engine.Flow, signal schema.Signal) {
	if signal.Name() == schema.SignalInterrupter {
		p.state = stateNormal
		p.backtickBuf.Reset()
		p.seenCloseBracket = false
	}
	flow.SendSignal(signal)
}

func (p *Plugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
	if payload.Name() != schema.PayloadLLMChunk {
		flow.SendPayload(payload)
		return
	}

	text := schema.GetAs(payload, "sentence", "")
	isFinal := schema.GetAs(payload, "is_final", false)

	filtered := p.filter(text)

	if strings.TrimSpace(filtered) == "" && !isFinal {
		return
	}

	out := schema.NewPayload(schema.PayloadLLMChunk)
	_ = out.Set("sentence", filtered)
	_ = out.Set("is_final", isFinal)
	flow.SendPayload(out.ReadOnly())
}

func (p *Plugin) filter(input string) string {
	var out strings.Builder
	runes := []rune(input)

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if r == '`' {
			p.backtickBuf.WriteRune(r)
			p.seenCloseBracket = false
			continue
		}

		if p.backtickBuf.Len() > 0 {
			count := p.backtickBuf.Len()
			p.backtickBuf.Reset()
			if count >= 3 {
				if p.state == stateInCodeBlock {
					p.state = stateNormal
				} else {
					p.state = stateInCodeBlock
				}
			}
			i--
			continue
		}

		if p.state == stateInCodeBlock {
			continue
		}

		if p.state == stateInLinkUrl {
			if r == ')' {
				p.state = stateNormal
			}
			continue
		}

		if r == '(' && p.seenCloseBracket {
			p.state = stateInLinkUrl
			p.seenCloseBracket = false
			continue
		}
		p.seenCloseBracket = false

		if r == '*' || r == '_' || r == '~' || r == '#' || r == '>' ||
			r == '[' || r == '\\' {
			continue
		}

		if r == ']' {
			p.seenCloseBracket = true
			continue
		}

		out.WriteRune(r)
	}

	return out.String()
}

func init() {
	if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
		Name: "markdown_filter",
		Inputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
		Outputs: engine.NewPropertyBuilder().
			AddPayload(schema.PayloadLLMChunk, "sentence", engine.TypeString, true).
			AddPayload(schema.PayloadLLMChunk, "is_final", engine.TypeBoolean, true).
			Build(),
	}); err != nil {
		panic(err)
	}
}
