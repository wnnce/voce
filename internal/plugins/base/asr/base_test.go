package asr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

type mockFlow struct {
	engine.Flow
	payloads []schema.Payload
}

func (m *mockFlow) SendPayload(p schema.Payload) {
	m.payloads = append(m.payloads, p)
}

func (m *mockFlow) SendSignal(_ schema.Signal)                                           {}
func (m *mockFlow) Publish(_ protocol.PacketType, _ []byte)                              {}
func (m *mockFlow) PublishFull(_ protocol.PacketType, _ protocol.PacketEncode, _ []byte) {}

func TestBasePlugin_HandleTranscription(t *testing.T) {
	flow := &mockFlow{}
	base := &BasePlugin{Flow: flow}

	// 1. Empty non-final transcription should be ignored
	base.HandleTranscription(&schema.Transcription{Text: "", Final: false})
	assert.Empty(t, flow.payloads)

	// 2. Non-empty transcription should be emitted
	tr := &schema.Transcription{
		Text:    "hello",
		Final:   true,
		Speaker: "speaker_0",
	}
	base.HandleTranscription(tr)
	require.Len(t, flow.payloads, 1)

	var unpacked schema.Transcription
	require.NoError(t, flow.payloads[0].Unpack(&unpacked))
	assert.Equal(t, "hello", unpacked.Text)
	assert.True(t, unpacked.Final)
	assert.Equal(t, "speaker_0", unpacked.Speaker)
}
