package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypes_BaseSchema_PackUnpack(t *testing.T) {
	orig := &BaseSchema{
		TurnID: 42,
		Metadata: map[string]any{
			"source": "mic",
			"score":  0.95,
		},
	}

	p := NewPayload("test_base")
	require.NoError(t, p.Pack(orig))

	var dst BaseSchema
	require.NoError(t, p.ReadOnly().Unpack(&dst))
	assert.Equal(t, orig.TurnID, dst.TurnID)
	assert.Equal(t, orig.Metadata, dst.Metadata)

	// Test with signal as well
	sig := NewSignal(SignalUserSpeechStart)
	require.NoError(t, sig.Pack(orig))

	var dstSig BaseSchema
	require.NoError(t, sig.ReadOnly().Unpack(&dstSig))
	assert.Equal(t, orig.TurnID, dstSig.TurnID)
	assert.Equal(t, orig.Metadata, dstSig.Metadata)
}

func TestTypes_Transcription_PackUnpack(t *testing.T) {
	orig := &Transcription{
		BaseSchema: BaseSchema{
			TurnID: 10,
			Metadata: map[string]any{
				"trace_id": "tr-123",
			},
		},
		Text:         "Hello world",
		Final:        true,
		Stable:       true,
		Speaker:      "speaker_0",
		Emotion:      "neutral",
		LanguageCode: "en-US",
	}

	p := NewPayload(PayloadASRResult)
	require.NoError(t, p.Pack(orig))

	var dst Transcription
	require.NoError(t, p.ReadOnly().Unpack(&dst))
	assert.Equal(t, *orig, dst)
}

func TestTypes_Caption_PackUnpack(t *testing.T) {
	orig := &Caption{
		BaseSchema: BaseSchema{
			TurnID: 11,
			Metadata: map[string]any{
				"render": true,
			},
		},
		Text:        "Hello world",
		Role:        RoleUser,
		IsFinal:     true,
		Speaker:     "speaker_1",
		Translation: "你好世界",
	}

	p := NewPayload(PayloadCaption)
	require.NoError(t, p.Pack(orig))

	var dst Caption
	require.NoError(t, p.ReadOnly().Unpack(&dst))
	assert.Equal(t, *orig, dst)
}

func TestTypes_LLMChunk_PackUnpack(t *testing.T) {
	orig := &LLMChunk{
		BaseSchema: BaseSchema{
			TurnID: 12,
		},
		Sentence: "partial response",
		IsFinal:  false,
	}

	p := NewPayload(PayloadLLMChunk)
	require.NoError(t, p.Pack(orig))

	var dst LLMChunk
	require.NoError(t, p.ReadOnly().Unpack(&dst))
	assert.Equal(t, *orig, dst)
}
