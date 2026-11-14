package soniox_asr

import (
	"context"
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

func TestSoniox_HandleResponse_ThreeStatesAndDiarization(t *testing.T) {
	p := NewPlugin(&SonioxConfig{
		EnableSpeakerDiarization: true,
		EnableEndpointDetection:  true,
	}).(*Plugin)
	flow := &mockFlow{}
	p.Flow = flow
	p.Ctx = context.Background()

	// 1. First message: partial hypothesis (is_final=false)
	resp1 := &serverResponse{
		Tokens: []token{
			{Text: "你好", IsFinal: false, Speaker: "1"},
		},
	}
	p.handleResponse(resp1)

	require.Len(t, flow.payloads, 1)
	var tr1 schema.Transcription
	require.NoError(t, flow.payloads[0].Unpack(&tr1))
	assert.Equal(t, "你好", tr1.Text)
	assert.False(t, tr1.Stable)
	assert.False(t, tr1.Final)
	assert.Equal(t, "speaker_1", tr1.Speaker)

	// 2. Second message: "你好" becomes final (stable chunk), "今天" is provisional
	flow.payloads = nil
	resp2 := &serverResponse{
		Tokens: []token{
			{Text: "你好", IsFinal: true, Speaker: "1"},
			{Text: "，今天", IsFinal: false, Speaker: "1"},
		},
	}
	p.handleResponse(resp2)

	require.Len(t, flow.payloads, 1)
	var tr2 schema.Transcription
	require.NoError(t, flow.payloads[0].Unpack(&tr2))
	assert.Equal(t, "你好，今天", tr2.Text)
	assert.False(t, tr2.Stable)
	assert.False(t, tr2.Final)

	// 3. Third message: "，今天天气很好" becomes finalized (stable chunk, not endpoint)
	flow.payloads = nil
	resp3 := &serverResponse{
		Tokens: []token{
			{Text: "，今天天气很好", IsFinal: true, Speaker: "1"},
		},
	}
	p.handleResponse(resp3)

	require.Len(t, flow.payloads, 1)
	var tr3 schema.Transcription
	require.NoError(t, flow.payloads[0].Unpack(&tr3))
	assert.Equal(t, "你好，今天天气很好", tr3.Text)
	assert.True(t, tr3.Stable)
	assert.False(t, tr3.Final)

	// 4. Fourth message: Endpoint detected with <end>
	flow.payloads = nil
	resp4 := &serverResponse{
		Tokens: []token{
			{Text: "<end>", IsFinal: true},
		},
	}
	p.handleResponse(resp4)

	require.Len(t, flow.payloads, 1)
	var tr4 schema.Transcription
	require.NoError(t, flow.payloads[0].Unpack(&tr4))
	assert.Equal(t, "你好，今天天气很好", tr4.Text)
	assert.True(t, tr4.Stable)
	assert.True(t, tr4.Final)
	assert.Equal(t, "speaker_1", tr4.Speaker)

	// Verify buffer was reset after endpoint
	assert.Empty(t, p.finalBuilder.String())
}
