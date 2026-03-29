package interrupter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

func TestInterrupter_Logic(t *testing.T) {
	// 1. Initialize
	ext := NewPlugin(engine.EmptyPluginConfig{})

	// 2. Setup Recording
	var receivedNames []string
	tester := engine.NewPluginTester(t, ext)
	tester.OnSignal(func(port int, signal schema.Signal) {
		receivedNames = append(receivedNames, signal.Name())
	})

	tester.Start()

	// Scenario: User starts speaking (!is_final)
	t.Log("Testing user start speaking (!is_final)...")
	p1 := schema.NewPayload(schema.PayloadASRResult)
	_ = p1.Set("is_final", false)
	tester.InjectPayload(p1.ReadOnly())

	// Scenario: User stops speaking (is_final)
	t.Log("Testing user stop speaking (is_final)...")
	p2 := schema.NewPayload(schema.PayloadASRResult)
	_ = p2.Set("is_final", true)
	tester.InjectPayload(p2.ReadOnly())

	// Verify Ordering
	// Expect: interrupter, user_speak_start (from first message)
	// Expect: user_speak_end (from second message)
	expected := []string{
		schema.SignalInterrupter,
		schema.SignalUserSpeechStart,
		schema.SignalUserSpeechEnd,
	}

	assert.Equal(t, expected, receivedNames)

	tester.Done()
	tester.Wait()
	tester.Stop()
}
