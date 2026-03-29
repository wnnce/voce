package sink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

func TestSink_OnSignal(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	testCases := []struct {
		signalName  string
		expectedMsg protocol.PacketType
	}{
		{schema.SignalInterrupter, protocol.TypeInterrupter},
		{schema.SignalUserSpeechStart, protocol.TypeUserSpeechStart},
		{schema.SignalUserSpeechEnd, protocol.TypeUserSpeechEnd},
		{schema.SignalAgentSpeechStart, protocol.TypeAgentSpeechStart},
		{schema.SignalAgentSpeechEnd, protocol.TypeAgentSpeechEnd},
	}

	for _, tc := range testCases {
		t.Run(tc.signalName, func(t *testing.T) {
			var capturedType protocol.PacketType
			tester.OnPublish(func(mt protocol.PacketType, data []byte) {
				capturedType = mt
				tester.Done()
			})

			tester.Start()
			tester.InjectSignal(schema.NewSignal(tc.signalName).ReadOnly())
			tester.Wait()

			assert.Equal(t, tc.expectedMsg, capturedType)
			tester.Stop()
		})
	}
}

func TestSink_OnAudio(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	var capturedData []byte
	var capturedType protocol.PacketType

	tester.OnPublish(func(mt protocol.PacketType, data []byte) {
		capturedType = mt
		capturedData = data
		tester.Done()
	})

	tester.Start()
	audioData := []byte("fake_audio_frame")
	a := schema.NewAudio(schema.AudioInput, engine.AudioSampleRate, engine.AudioChannels)
	a.SetBytes(audioData)
	tester.InjectAudio(a.ReadOnly())
	tester.Wait()

	assert.Equal(t, protocol.TypeAudio, capturedType)
	assert.Equal(t, audioData, capturedData)
	tester.Stop()
}

func TestSink_OnData_Subtitle(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	var capturedData []byte
	var capturedType protocol.PacketType

	tester.OnPublishFull(func(mt protocol.PacketType, ed protocol.PacketEncode, data []byte) {
		capturedType = mt
		capturedData = data
		tester.Done()
	})

	tester.Start()
	// Create caption data
	subContent := []byte(`{"text": "hello"}`)
	d := schema.NewPayload(schema.PayloadCaption)
	_ = d.Set("caption", subContent)

	tester.InjectPayload(d.ReadOnly())
	tester.Wait()

	assert.Equal(t, protocol.TypeCaption, capturedType)
	assert.Equal(t, subContent, capturedData)
	tester.Stop()
}

func TestSink_IgnoreUnknownSignal(t *testing.T) {
	ext := NewPlugin(engine.EmptyPluginConfig{})
	tester := engine.NewPluginTester(t, ext)

	emitted := false
	tester.OnPublish(func(mt protocol.PacketType, data []byte) {
		emitted = true
	})

	tester.Start()
	// Inject a command not handled by sink (like a generic one)
	tester.InjectSignal(schema.NewSignal("unknown_command").ReadOnly())

	// We expect no activity, so it should timeout (wait handles that)
	// or we just call Stop after some silence.
	// For sink, if it doesn't match default, it just returns.
	tester.Stop()

	assert.False(t, emitted)
}
