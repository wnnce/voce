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
	type signalRecord struct {
		name   string
		turnID uint64
	}
	var receivedSignals []signalRecord
	var receivedPayloadTurnIDs []uint64

	tester := engine.NewPluginTester(t, ext)
	tester.OnSignal(func(port int, signal schema.Signal) {
		turnID := schema.GetAs(signal, "turn_id", uint64(0))
		receivedSignals = append(receivedSignals, signalRecord{
			name:   signal.Name(),
			turnID: turnID,
		})
	})
	tester.OnPayload(func(port int, payload schema.Payload) {
		turnID := schema.GetAs(payload, "turn_id", uint64(0))
		receivedPayloadTurnIDs = append(receivedPayloadTurnIDs, turnID)
	})

	tester.Start()

	// Turn 1: User starts speaking (!is_final)
	t.Log("Testing user start speaking (!is_final)...")
	p1 := schema.NewPayload(schema.PayloadASRResult)
	_ = p1.Set("is_final", false)
	tester.InjectPayload(p1.ReadOnly())

	// Turn 1: Intermediate chunk (!is_final)
	p2 := schema.NewPayload(schema.PayloadASRResult)
	_ = p2.Set("is_final", false)
	tester.InjectPayload(p2.ReadOnly())

	// Turn 1: User stops speaking (is_final)
	t.Log("Testing user stop speaking (is_final)...")
	p3 := schema.NewPayload(schema.PayloadASRResult)
	_ = p3.Set("is_final", true)
	tester.InjectPayload(p3.ReadOnly())

	// Turn 2: User speaks in a single packet directly is_final == true
	t.Log("Testing one-shot speech directly is_final...")
	p4 := schema.NewPayload(schema.PayloadASRResult)
	_ = p4.Set("is_final", true)
	tester.InjectPayload(p4.ReadOnly())

	tester.Done()
	tester.Wait()
	tester.Stop()

	// Verify Ordering and turn_ids:
	// Turn 1: interrupter(1), user_speech_start(1), user_speech_end(1)
	// Turn 2: interrupter(2), user_speech_start(2), user_speech_end(2)
	expectedSignals := []signalRecord{
		{name: schema.SignalInterrupter, turnID: 1},
		{name: schema.SignalUserSpeechStart, turnID: 1},
		{name: schema.SignalUserSpeechEnd, turnID: 1},
		{name: schema.SignalInterrupter, turnID: 2},
		{name: schema.SignalUserSpeechStart, turnID: 2},
		{name: schema.SignalUserSpeechEnd, turnID: 2},
	}

	assert.Equal(t, expectedSignals, receivedSignals)
	assert.Equal(t, []uint64{1, 1, 1, 2}, receivedPayloadTurnIDs)
}
