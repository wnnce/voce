package deepgram_asr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

func TestDeepgram_BuildURL(t *testing.T) {
	cfg := &DeepgramConfig{
		ApiKey:   "test-key",
		Model:    "nova-3",
		Language: "en-US",
		Diarize:  true,
	}
	p := NewPlugin(cfg).(*Plugin)
	u, err := p.buildURL()
	require.NoError(t, err)
	assert.Contains(t, u, "diarize=true")
	assert.Contains(t, u, "model=nova-3")
	assert.Contains(t, u, "language=en-US")

	cfgNoDiarize := &DeepgramConfig{
		ApiKey:  "test-key",
		Diarize: false,
	}
	pNoDiarize := NewPlugin(cfgNoDiarize).(*Plugin)
	uNoDiarize, err := pNoDiarize.buildURL()
	require.NoError(t, err)
	assert.NotContains(t, uNoDiarize, "diarize=true")
}

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

func TestDeepgram_HandleResults_ThreeStatesAndDiarization(t *testing.T) {
	spk1 := 1
	tests := []struct {
		name          string
		diarize       bool
		msg           *serverMessage
		expectedText  string
		expectFinal   bool
		expectStable  bool
		expectSpeaker string
	}{
		{
			name:    "interim result (not stable, not final)",
			diarize: true,
			msg: &serverMessage{
				Type:        typeResults,
				IsFinal:     false,
				SpeechFinal: false,
				Channel: &channel{
					Alternatives: []alternative{
						{
							Transcript: "Hello",
							Words: []word{
								{Word: "Hello", Speaker: &spk1},
							},
						},
					},
				},
			},
			expectedText:  "Hello",
			expectFinal:   false,
			expectStable:  false,
			expectSpeaker: "speaker_1",
		},
		{
			name:    "stable chunk (stable, but not speech_final)",
			diarize: true,
			msg: &serverMessage{
				Type:        typeResults,
				IsFinal:     true,
				SpeechFinal: false,
				Channel: &channel{
					Alternatives: []alternative{
						{
							Transcript: "Hello world",
							Words: []word{
								{Word: "Hello", Speaker: &spk1},
								{Word: "world", Speaker: &spk1},
							},
						},
					},
				},
			},
			expectedText:  "Hello world",
			expectFinal:   false,
			expectStable:  true,
			expectSpeaker: "speaker_1",
		},
		{
			name:    "utterance final (stable and speech_final)",
			diarize: true,
			msg: &serverMessage{
				Type:        typeResults,
				IsFinal:     true,
				SpeechFinal: true,
				Channel: &channel{
					Alternatives: []alternative{
						{
							Transcript: "Hello world.",
							Words: []word{
								{Word: "Hello", Speaker: &spk1},
								{Word: "world.", Speaker: &spk1},
							},
						},
					},
				},
			},
			expectedText:  "Hello world.",
			expectFinal:   true,
			expectStable:  true,
			expectSpeaker: "speaker_1",
		},
		{
			name:    "no diarization configured",
			diarize: false,
			msg: &serverMessage{
				Type:        typeResults,
				IsFinal:     true,
				SpeechFinal: true,
				Channel: &channel{
					Alternatives: []alternative{
						{
							Transcript: "Plain transcript",
						},
					},
				},
			},
			expectedText:  "Plain transcript",
			expectFinal:   true,
			expectStable:  true,
			expectSpeaker: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPlugin(&DeepgramConfig{Diarize: tt.diarize}).(*Plugin)
			flow := &mockFlow{}
			p.Flow = flow
			p.Ctx = context.Background()

			p.handleResults(tt.msg)

			require.Len(t, flow.payloads, 1)
			var tr schema.Transcription
			err := flow.payloads[0].Unpack(&tr)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedText, tr.Text)
			assert.Equal(t, tt.expectFinal, tr.Final)
			assert.Equal(t, tt.expectStable, tr.Stable)
			assert.Equal(t, tt.expectSpeaker, tr.Speaker)
		})
	}
}
