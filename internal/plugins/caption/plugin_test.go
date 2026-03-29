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
