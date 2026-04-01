package md_filter

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

func TestMdFilter_Basic(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	var lastText string
	tester.OnPayload(func(port int, payload schema.Payload) {
		lastText = schema.GetAs(payload, "sentence", "")
	})

	tester.Start()

	// 1. Simple Bold/Italic/Nesting
	d1 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d1.Set("sentence", "Hello **World** and _Rust_ with __nesting__.")
	tester.InjectPayload(d1.ReadOnly())
	assert.Equal(t, "Hello World and Rust with nesting.", lastText)

	// 2. Links
	d2 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d2.Set("sentence", "Visit [Google](https://google.com).")
	tester.InjectPayload(d2.ReadOnly())
	assert.Equal(t, "Visit Google.", lastText)

	tester.Done()
	tester.Wait()
	tester.Stop()
}

func TestMdFilter_CrossChunkLink(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	var lastText string
	tester.OnPayload(func(port int, payload schema.Payload) {
		lastText = schema.GetAs(payload, "sentence", "")
	})

	tester.Start()

	// Chunk 1: [Link
	d1 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d1.Set("sentence", "[Link")
	tester.InjectPayload(d1.ReadOnly())
	assert.Equal(t, "Link", lastText)

	// Chunk 2: text]
	d2 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d2.Set("sentence", " text]")
	tester.InjectPayload(d2.ReadOnly())
	assert.Equal(t, " text", lastText)

	// Chunk 3: (https://example.com)
	d3 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d3.Set("sentence", "(https://example.com) is here.")
	_ = d3.Set("is_final", true)
	tester.InjectPayload(d3.ReadOnly())

	tester.Done()
	tester.Wait()
	assert.Equal(t, " is here.", lastText)
	tester.Stop()
}

func TestMdFilter_CodeBlock(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	var lastText string
	tester.OnPayload(func(port int, payload schema.Payload) {
		lastText = schema.GetAs(payload, "sentence", "")
	})

	tester.Start()

	d1 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d1.Set("sentence", "Code: ```go\n")
	tester.InjectPayload(d1.ReadOnly())

	d2 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d2.Set("sentence", "println(\"hi\")\n``` and post.")
	_ = d2.Set("is_final", true)
	tester.InjectPayload(d2.ReadOnly())

	tester.Done()
	tester.Wait()
	assert.Equal(t, " and post.", lastText)
	tester.Stop()
}

func TestMdFilter_Interruption(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	var lastText string
	tester.OnPayload(func(port int, payload schema.Payload) {
		lastText = schema.GetAs(payload, "sentence", "")
	})

	tester.Start()

	d1 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d1.Set("sentence", "```broken block")
	tester.InjectPayload(d1.ReadOnly())

	tester.InjectSignal(schema.NewSignal(schema.SignalInterrupter).ReadOnly())

	d2 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d2.Set("sentence", "Normal text")
	_ = d2.Set("is_final", true)
	tester.InjectPayload(d2.ReadOnly())

	tester.Done()
	tester.Wait()
	assert.Equal(t, "Normal text", lastText)
	tester.Stop()
}

// --- Benchmarks ---

func BenchmarkFilterNormalText(b *testing.B) {
	p := &Plugin{}
	text := "This is a normal sentence without any markdown symbols."
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		p.filter(text)
	}
}

func BenchmarkFilterHeavyMarkdown(b *testing.B) {
	p := &Plugin{}
	text := "Hello **World**, [this](https://example.com) is a _heavy_ markdown `text` with # headers and > quotes."
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		p.filter(text)
	}
}

func BenchmarkFilterLargeCodeBlock(b *testing.B) {
	p := &Plugin{}
	var sb strings.Builder
	sb.WriteString("Start of code block: ```go\n")
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf("func test%d() { println(\"testing\") }\n", i))
	}
	sb.WriteString("``` End of code block.")
	text := sb.String()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		p.filter(text)
	}
}

func BenchmarkFilterLongLinks(b *testing.B) {
	p := &Plugin{}
	text := "[Link Text](https://very-long-url-with-lots-of-parameters.com/v1/resource?id=123456789&token=abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ)"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		p.filter(text)
	}
}
