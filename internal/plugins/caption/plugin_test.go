package caption

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

func TestCaption_Framework(t *testing.T) {
	// 1. Initialize Plugin
	ext := NewPlugin(engine.EmptyPluginConfig{})

	// 2. Setup Variable to capture results
	var finalSub Caption

	// 3. Harness the extension using VETF
	tester := engine.NewPluginTester(t, ext)
	tester.OnPayload(func(port int, payload schema.Payload) {
		var sub Caption
		if payload.Name() != schema.PayloadCaption {
			return
		}
		err := payload.Bind("caption", &sub)
		require.NoError(t, err)
		if sub.IsFinal {
			finalSub = sub
			tester.Done()
		}
	})

	// 5. Start the lifecycle
	tester.Start()

	// 6. Simulate streaming data (e.g. LLM tokens)
	d1 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d1.Set("sentence", "Hello ")
	_ = d1.Set("is_final", false)
	tester.InjectPayload(d1.ReadOnly())

	d2 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d2.Set("sentence", "world")
	_ = d2.Set("is_final", true)
	tester.InjectPayload(d2.ReadOnly())

	// 7. Block until Done() or 10s Timeout
	tester.Wait()

	// 8. Assert end state
	assert.Equal(t, roleAssistant, finalSub.Role)
	assert.Equal(t, "Hello world", finalSub.Text)
	assert.True(t, finalSub.IsFinal)

	// 9. Resource Cleanup
	tester.Stop()
}

func TestCaption_ResetOnInterruption(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	var lastReceived string
	tester := engine.NewPluginTester(t, ext)
	tester.OnPayload(func(port int, payload schema.Payload) {
		if payload.Name() != schema.PayloadCaption {
			return
		}
		var sub Caption
		err := payload.Bind("caption", &sub)
		require.NoError(t, err)
		lastReceived = sub.Text
	})

	tester.Start()

	// Partial sentence
	d1 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = d1.Set("sentence", "Waiting for ")
	tester.InjectPayload(d1.ReadOnly())

	// Interruption mid-speech
	tester.InjectSignal(schema.NewSignal(schema.SignalInterrupter).ReadOnly())

	// Next sentence should not contain "Waiting for "
	d2 := schema.NewPayload(schema.PayloadASRResult)
	_ = d2.Set("text", "Starting fresh")
	_ = d2.Set("is_final", true)
	tester.InjectPayload(d2.ReadOnly())

	tester.Done()
	tester.Wait()

	assert.Equal(t, "Starting fresh", lastReceived)
	tester.Stop()
}

func TestCaption_BufferAssistantUntilUserFinal(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	var received []Caption
	tester := engine.NewPluginTester(t, ext)
	tester.OnPayload(func(port int, payload schema.Payload) {
		if payload.Name() != schema.PayloadCaption {
			return
		}
		var sub Caption
		err := payload.Bind("caption", &sub)
		require.NoError(t, err)
		received = append(received, sub)
	})

	tester.Start()

	userPartial := schema.NewPayload(schema.PayloadASRResult)
	_ = userPartial.Set("text", "今天")
	_ = userPartial.Set("is_final", false)
	tester.InjectPayload(userPartial.ReadOnly())

	assistantDelta := schema.NewPayload(schema.PayloadLLMChunk)
	_ = assistantDelta.Set("sentence", "你好")
	_ = assistantDelta.Set("is_final", false)
	tester.InjectPayload(assistantDelta.ReadOnly())

	assistantDelta2 := schema.NewPayload(schema.PayloadLLMChunk)
	_ = assistantDelta2.Set("sentence", "，我是")
	_ = assistantDelta2.Set("is_final", false)
	tester.InjectPayload(assistantDelta2.ReadOnly())

	require.Len(t, received, 1)
	assert.Equal(t, roleUser, received[0].Role)
	assert.Equal(t, "今天", received[0].Text)
	assert.False(t, received[0].IsFinal)

	userFinal := schema.NewPayload(schema.PayloadASRResult)
	_ = userFinal.Set("text", "今天天气怎么样")
	_ = userFinal.Set("is_final", true)
	tester.InjectPayload(userFinal.ReadOnly())

	assistantDone := schema.NewPayload(schema.PayloadLLMChunk)
	_ = assistantDone.Set("sentence", "，有什么可以帮你？")
	_ = assistantDone.Set("is_final", true)
	tester.InjectPayload(assistantDone.ReadOnly())

	require.Len(t, received, 4)
	assert.Equal(t, Caption{Text: "今天天气怎么样", Role: roleUser, IsFinal: true}, received[1])
	assert.Equal(t, Caption{Text: "你好，我是", Role: roleAssistant, IsFinal: false}, received[2])
	assert.Equal(t, Caption{Text: "你好，我是，有什么可以帮你？", Role: roleAssistant, IsFinal: true}, received[3])

	tester.Stop()
}
