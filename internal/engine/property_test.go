package engine

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProperty_String(t *testing.T) {
	p := Property{
		Prefix: PrefixAudio,
		Name:   "micro",
	}
	assert.Equal(t, "audio:micro", p.String())
}

func TestValidateProperty(t *testing.T) {
	t.Run("Exact match", func(t *testing.T) {
		up := Property{
			Prefix: PrefixPayload,
			Name:   "text",
			Fields: []Field{{Key: "content", Type: TypeString, Required: true}},
		}
		down := Property{
			Prefix: PrefixPayload,
			Name:   "text",
			Fields: []Field{{Key: "content", Type: TypeString, Required: true}},
		}
		assert.NoError(t, ValidateProperty(up, down))
	})

	t.Run("Type wildcard (TypeAny) accepts any upstream type", func(t *testing.T) {
		up := Property{
			Prefix: PrefixAudio,
			Name:   "stream",
			Fields: []Field{{Key: "format", Type: PropertyType("pcm_16k"), Required: true}},
		}
		down := Property{
			Prefix: PrefixAudio,
			Name:   "stream",
			Fields: []Field{{Key: "format", Type: TypeAny, Required: true}},
		}
		assert.NoError(t, ValidateProperty(up, down))
	})

	t.Run("Type mismatch", func(t *testing.T) {
		up := Property{
			Prefix: PrefixPayload,
			Name:   "result",
			Fields: []Field{{Key: "score", Type: TypeInteger, Required: true}},
		}
		down := Property{
			Prefix: PrefixPayload,
			Name:   "result",
			Fields: []Field{{Key: "score", Type: TypeString, Required: true}},
		}
		err := ValidateProperty(up, down)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type mismatch")
	})

	t.Run("Required field missing in upstream", func(t *testing.T) {
		up := Property{Prefix: PrefixPayload, Name: "info"}
		down := Property{
			Prefix: PrefixPayload,
			Name:   "info",
			Fields: []Field{{Key: "id", Type: TypeString, Required: true}},
		}
		err := ValidateProperty(up, down)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not provided by upstream")
	})

	t.Run("Optional upstream cannot satisfy required downstream field", func(t *testing.T) {
		up := Property{
			Prefix: PrefixSignal,
			Name:   "control",
			Fields: []Field{{Key: "action", Type: TypeString, Required: false}},
		}
		down := Property{
			Prefix: PrefixSignal,
			Name:   "control",
			Fields: []Field{{Key: "action", Type: TypeString, Required: true}},
		}
		err := ValidateProperty(up, down)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only optionally produced")
	})
}

func TestValidateProperties(t *testing.T) {
	t.Run("Success if target node does not accept prefix (Tolerant mode)", func(t *testing.T) {
		downs := NewPropertyBuilder().AddSignalEvent("start").Build()
		err := ValidateProperties(nil, downs, PrefixPayload)
		assert.NoError(t, err)
	})

	t.Run("Fails if source node does not produce prefix", func(t *testing.T) {
		ups := NewPropertyBuilder().AddSignalEvent("start").Build()
		downs := NewPropertyBuilder().AddPayload("text", "content", TypeString, true).Build()
		err := ValidateProperties(ups, downs, PrefixPayload)
		assert.Contains(t, err.Error(), "source node does not produce [data]")
	})

	t.Run("Fails if no recognized properties (of same prefix) in upstreams", func(t *testing.T) {
		ups := NewPropertyBuilder().AddPayload("unknown", "val", TypeAny, true).Build()
		downs := NewPropertyBuilder().AddPayload("text", "content", TypeString, true).Build()
		err := ValidateProperties(ups, downs, PrefixPayload)
		assert.Contains(t, err.Error(), "none of the source [data] properties are recognized")
	})

	t.Run("Success if at least one property matches (Subtitle case)", func(t *testing.T) {
		// Upstream only provides ASR
		ups := NewPropertyBuilder().
			AddPayload("asr_result", "text", TypeString, true).
			Build()
		// Downstream supports both ASR and LLM
		downs := NewPropertyBuilder().
			AddPayload("asr_result", "text", TypeString, true).
			AddPayload("llm_chunk", "sentence", TypeString, true).
			Build()
		assert.NoError(t, ValidateProperties(ups, downs, PrefixPayload))
	})

	t.Run("Contract violation on a recognized property fails entire set", func(t *testing.T) {
		ups := NewPropertyBuilder().
			AddPayload("asr_result", "text", TypeString, true).     // Correct
			AddPayload("llm_chunk", "sentence", TypeInteger, true). // Broken
			Build()
		downs := NewPropertyBuilder().
			AddPayload("asr_result", "text", TypeString, true).
			AddPayload("llm_chunk", "sentence", TypeString, true).
			Build()
		err := ValidateProperties(ups, downs, PrefixPayload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contract violation")
	})

	t.Run("Unrecognized upstream properties (same prefix) are ignored", func(t *testing.T) {
		ups := NewPropertyBuilder().
			AddPayload("asr_result", "text", TypeString, true).
			AddPayload("garbage", "data", TypeAny, false).
			Build()
		downs := NewPropertyBuilder().
			AddPayload("asr_result", "text", TypeString, true).
			Build()
		assert.NoError(t, ValidateProperties(ups, downs, PrefixPayload))
	})

	t.Run("Wildcard support for recognized upstreams", func(t *testing.T) {
		ups := NewPropertyBuilder().
			AddPayload("anything", "content", TypeString, true).
			Build()
		downs := NewPropertyBuilder().
			AddWildPayload("content", TypeString, true).
			Build()
		assert.NoError(t, ValidateProperties(ups, downs, PrefixPayload))
	})
}

func TestPropertyBuilder(t *testing.T) {
	t.Run("Signal-only has no fields", func(t *testing.T) {
		props := NewPropertyBuilder().
			AddSignalEvent("start").
			Build()
		require.Len(t, props, 1)
		assert.Equal(t, PrefixSignal, props[0].Prefix)
		assert.Equal(t, "start", props[0].Name)
		assert.Empty(t, props[0].Fields)
	})

	t.Run("Multiple fields collapsed into one property", func(t *testing.T) {
		props := NewPropertyBuilder().
			AddPayload("text", "content", TypeString, true).
			AddPayload("text", "final", TypeBoolean, false).
			Build()
		require.Len(t, props, 1)
		assert.Len(t, props[0].Fields, 2)
	})

	t.Run("Different names produce separate properties", func(t *testing.T) {
		props := NewPropertyBuilder().
			AddPayload("transcription", "text", TypeString, true).
			AddPayload("sentiment", "label", TypeString, false).
			Build()
		assert.Len(t, props, 2)
	})

	t.Run("Mixed prefixes and names", func(t *testing.T) {
		props := NewPropertyBuilder().
			AddSignalEvent("ready").
			AddSignal("flush", "force", TypeBoolean, true).
			AddAudio("stream", "format", PropertyType("pcm"), false).
			Build()
		assert.Len(t, props, 3)
		assert.Equal(t, PrefixSignal, props[0].Prefix)
		assert.Equal(t, PrefixSignal, props[1].Prefix)
		assert.Equal(t, PrefixAudio, props[2].Prefix)
	})
}

func BenchmarkValidateProperties(b *testing.B) {
	ups, downs := setupBenchmarkData(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateProperties(ups, downs, PrefixPayload)
	}
}

func setupBenchmarkData(count int) ([]Property, []Property) {
	ub := NewPropertyBuilder()
	db := NewPropertyBuilder()
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("prop_%d", i)
		ub.AddPayload(name, "value", TypeString, true)
		db.AddPayload(name, "value", TypeString, true)
	}
	return ub.Build(), db.Build()
}
