package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testManualChunk struct {
	Sentence string
	IsFinal  bool
	Emotion  string
}

func (c *testManualChunk) PackSchema(props Properties) error {
	if err := props.Set("sentence", c.Sentence); err != nil {
		return err
	}
	if err := props.Set("is_final", c.IsFinal); err != nil {
		return err
	}
	if c.Emotion != "" {
		if err := props.Set("emotion", c.Emotion); err != nil {
			return err
		}
	}
	return nil
}

func (c *testManualChunk) UnpackSchema(data ReadOnly) error {
	c.Sentence = GetAs[string](data, "sentence", "")
	c.IsFinal = GetAs[bool](data, "is_final", false)
	c.Emotion = GetAs[string](data, "emotion", "")
	return nil
}

type testReflectChunk struct {
	Sentence string `schema:"sentence,required"`
	IsFinal  bool   `schema:"is_final"`
	Emotion  string `schema:"emotion,omitzero"`
}

type testOmitEmptyStruct struct {
	Tags []string `schema:"tags,omitempty"`
}

type testOmitZeroStruct struct {
	Tags []string `schema:"tags,omitzero"`
}

type testRequiredStruct struct {
	Name string `schema:"name,required"`
	Age  int    `schema:"age"`
}

type testSkipAndUnexported struct {
	Visible    string `schema:"visible"`
	Skipped    string `schema:"-"`
	NoTag      string
	unexported string `schema:"hidden"` //nolint:unused // reserved fields
}

type testComplexField struct {
	Info testInner `schema:"info"`
}

type testInner struct {
	Key   string `json:"key"`
	Value int    `json:"value"`
}

func TestPack_Interface(t *testing.T) {
	t.Run("pack via Packer interface", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		chunk := &testManualChunk{Sentence: "hello", IsFinal: true, Emotion: "happy"}

		require.NoError(t, payload.Pack(chunk))

		assert.Equal(t, "hello", GetAs[string](payload, "sentence"))
		assert.True(t, GetAs[bool](payload, "is_final"))
		assert.Equal(t, "happy", GetAs[string](payload, "emotion"))
	})

	t.Run("omit emotion when empty", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		chunk := &testManualChunk{Sentence: "hello"}

		require.NoError(t, payload.Pack(chunk))

		_, ok := payload.Get("emotion")
		assert.False(t, ok)
	})

	t.Run("readonly rejects pack", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		ro := payload.ReadOnly()

		err := ro.(Properties).Pack(&testManualChunk{Sentence: "hello"})
		assert.ErrorIs(t, err, ErrReadOnly)
	})
}

func TestUnpack_Interface(t *testing.T) {
	t.Run("unpack via Unpacker interface", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		_ = payload.Set("sentence", "world")
		_ = payload.Set("is_final", true)
		_ = payload.Set("emotion", "calm")
		ro := payload.ReadOnly()

		var chunk testManualChunk
		require.NoError(t, ro.Unpack(&chunk))

		assert.Equal(t, "world", chunk.Sentence)
		assert.True(t, chunk.IsFinal)
		assert.Equal(t, "calm", chunk.Emotion)
	})
}

func TestPack_Reflect(t *testing.T) {
	t.Run("basic pack via struct tags", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		chunk := &testReflectChunk{Sentence: "hello", IsFinal: true, Emotion: "happy"}

		require.NoError(t, payload.Pack(chunk))

		assert.Equal(t, "hello", GetAs[string](payload, "sentence"))
		assert.True(t, GetAs[bool](payload, "is_final"))
		assert.Equal(t, "happy", GetAs[string](payload, "emotion"))
	})

	t.Run("pack non-pointer struct", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		chunk := testReflectChunk{Sentence: "value", IsFinal: false}

		require.NoError(t, payload.Pack(chunk))

		assert.Equal(t, "value", GetAs[string](payload, "sentence"))
	})

	t.Run("omitzero skips zero-value string", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		chunk := &testReflectChunk{Sentence: "hello"}

		require.NoError(t, payload.Pack(chunk))

		_, ok := payload.Get("emotion")
		assert.False(t, ok, "emotion with omitzero should be skipped when zero")

		// is_final has no omit tag, zero value should still be written
		val, ok := payload.Get("is_final")
		assert.True(t, ok, "is_final without omit tag should be written")
		assert.Equal(t, false, val)
	})

	t.Run("readonly rejects reflect pack", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		ro := payload.ReadOnly()

		err := ro.(Properties).Pack(&testReflectChunk{Sentence: "hello"})
		assert.ErrorIs(t, err, ErrReadOnly)
	})

	t.Run("non-struct returns error", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		err := payload.Pack("not a struct")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a struct")
	})

	t.Run("nil pointer returns error", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		err := payload.Pack((*testReflectChunk)(nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil pointer")
	})
}

func TestUnpack_Reflect(t *testing.T) {
	t.Run("basic unpack via struct tags", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		_ = payload.Set("sentence", "world")
		_ = payload.Set("is_final", true)
		_ = payload.Set("emotion", "calm")
		ro := payload.ReadOnly()

		var chunk testReflectChunk
		require.NoError(t, ro.Unpack(&chunk))

		assert.Equal(t, "world", chunk.Sentence)
		assert.True(t, chunk.IsFinal)
		assert.Equal(t, "calm", chunk.Emotion)
	})

	t.Run("missing optional fields use zero values", func(t *testing.T) {
		payload := NewPayload(PayloadLLMChunk)
		_ = payload.Set("sentence", "hello")
		ro := payload.ReadOnly()

		var chunk testReflectChunk
		require.NoError(t, ro.Unpack(&chunk))

		assert.Equal(t, "hello", chunk.Sentence)
		assert.False(t, chunk.IsFinal)
		assert.Empty(t, chunk.Emotion)
	})

	t.Run("required field missing returns error", func(t *testing.T) {
		payload := NewPayload("test")
		_ = payload.Set("age", 25)
		ro := payload.ReadOnly()

		var s testRequiredStruct
		err := ro.Unpack(&s)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("required field present succeeds", func(t *testing.T) {
		payload := NewPayload("test")
		_ = payload.Set("name", "alice")
		ro := payload.ReadOnly()

		var s testRequiredStruct
		require.NoError(t, ro.Unpack(&s))
		assert.Equal(t, "alice", s.Name)
		assert.Equal(t, 0, s.Age)
	})

	t.Run("non-pointer returns error", func(t *testing.T) {
		payload := NewPayload("test")
		ro := payload.ReadOnly()

		var chunk testReflectChunk
		err := ro.Unpack(chunk) // not a pointer
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-nil pointer")
	})
}

func TestOmitEmpty_vs_OmitZero(t *testing.T) {
	t.Run("omitempty: nil slice is skipped", func(t *testing.T) {
		payload := NewPayload("test")
		s := &testOmitEmptyStruct{Tags: nil}

		require.NoError(t, payload.Pack(s))

		_, ok := payload.Get("tags")
		assert.False(t, ok)
	})

	t.Run("omitempty: empty non-nil slice is kept", func(t *testing.T) {
		payload := NewPayload("test")
		s := &testOmitEmptyStruct{Tags: []string{}}

		require.NoError(t, payload.Pack(s))

		_, ok := payload.Get("tags")
		assert.True(t, ok, "omitempty should keep empty non-nil slice")
	})

	t.Run("omitzero: nil slice is skipped", func(t *testing.T) {
		payload := NewPayload("test")
		s := &testOmitZeroStruct{Tags: nil}

		require.NoError(t, payload.Pack(s))

		_, ok := payload.Get("tags")
		assert.False(t, ok)
	})

	t.Run("omitzero: empty non-nil slice is also skipped", func(t *testing.T) {
		payload := NewPayload("test")
		s := &testOmitZeroStruct{Tags: []string{}}

		require.NoError(t, payload.Pack(s))

		_, ok := payload.Get("tags")
		assert.False(t, ok, "omitzero should skip empty non-nil slice too")
	})

	t.Run("omitzero: non-empty slice is kept", func(t *testing.T) {
		payload := NewPayload("test")
		s := &testOmitZeroStruct{Tags: []string{"go"}}

		require.NoError(t, payload.Pack(s))

		_, ok := payload.Get("tags")
		assert.True(t, ok)
	})
}

func TestSkipAndUnexported(t *testing.T) {
	t.Run("fields with - tag and no tag are ignored", func(t *testing.T) {
		payload := NewPayload("test")
		s := &testSkipAndUnexported{
			Visible: "yes",
			Skipped: "no",
			NoTag:   "also no",
		}

		require.NoError(t, payload.Pack(s))

		val, ok := payload.Get("visible")
		assert.True(t, ok)
		assert.Equal(t, "yes", val)

		_, ok = payload.Get("skipped")
		assert.False(t, ok, "field with - tag should be skipped")

		// NoTag has no schema tag, should not be packed
		for _, e := range payload.(*builtinPayload).entries {
			assert.NotEqual(t, "NoTag", e.key)
		}
	})
}

func TestComplexField(t *testing.T) {
	t.Run("struct field round-trip via sonic", func(t *testing.T) {
		payload := NewPayload("test")
		src := &testComplexField{Info: testInner{Key: "foo", Value: 42}}

		require.NoError(t, payload.Pack(src))

		ro := payload.ReadOnly()
		var dst testComplexField
		require.NoError(t, ro.Unpack(&dst))

		assert.Equal(t, "foo", dst.Info.Key)
		assert.Equal(t, 42, dst.Info.Value)
	})
}

func TestPackUnpackRoundTrip(t *testing.T) {
	t.Run("interface round-trip", func(t *testing.T) {
		original := &testManualChunk{Sentence: "roundtrip", IsFinal: true, Emotion: "excited"}

		payload := NewPayload(PayloadLLMChunk)
		require.NoError(t, payload.Pack(original))
		ro := payload.ReadOnly()

		var restored testManualChunk
		require.NoError(t, ro.Unpack(&restored))

		assert.Equal(t, original.Sentence, restored.Sentence)
		assert.Equal(t, original.IsFinal, restored.IsFinal)
		assert.Equal(t, original.Emotion, restored.Emotion)
	})

	t.Run("reflection round-trip", func(t *testing.T) {
		original := &testReflectChunk{Sentence: "reflect", IsFinal: true, Emotion: "calm"}

		payload := NewPayload(PayloadLLMChunk)
		require.NoError(t, payload.Pack(original))
		ro := payload.ReadOnly()

		var restored testReflectChunk
		require.NoError(t, ro.Unpack(&restored))

		assert.Equal(t, original.Sentence, restored.Sentence)
		assert.Equal(t, original.IsFinal, restored.IsFinal)
		assert.Equal(t, original.Emotion, restored.Emotion)
	})

	t.Run("reflection with omitted fields", func(t *testing.T) {
		original := &testReflectChunk{Sentence: "partial"}

		payload := NewPayload(PayloadLLMChunk)
		require.NoError(t, payload.Pack(original))
		ro := payload.ReadOnly()

		var restored testReflectChunk
		require.NoError(t, ro.Unpack(&restored))

		assert.Equal(t, "partial", restored.Sentence)
		assert.False(t, restored.IsFinal)
		assert.Empty(t, restored.Emotion)
	})
}

func BenchmarkPackUnpack(b *testing.B) {
	// Interface path: Packer/Unpacker
	b.Run("pack/interface", func(b *testing.B) {
		b.ReportAllocs()
		chunk := &testManualChunk{Sentence: "hello world", IsFinal: true, Emotion: "happy"}
		b.ResetTimer()
		for b.Loop() {
			payload := NewPayload(PayloadLLMChunk)
			_ = payload.Pack(chunk)
		}
	})

	b.Run("pack/reflect", func(b *testing.B) {
		b.ReportAllocs()
		chunk := &testReflectChunk{Sentence: "hello world", IsFinal: true, Emotion: "happy"}
		b.ResetTimer()
		for b.Loop() {
			payload := NewPayload(PayloadLLMChunk)
			_ = payload.Pack(chunk)
		}
	})

	b.Run("pack/manual_set", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			payload := NewPayload(PayloadLLMChunk)
			_ = payload.Set("sentence", "hello world")
			_ = payload.Set("is_final", true)
			_ = payload.Set("emotion", "happy")
		}
	})

	// Unpack benchmarks need pre-populated payloads
	preparedPayload := NewPayload(PayloadLLMChunk)
	_ = preparedPayload.Set("sentence", "hello world")
	_ = preparedPayload.Set("is_final", true)
	_ = preparedPayload.Set("emotion", "happy")
	ro := preparedPayload.ReadOnly()

	b.Run("unpack/interface", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			var chunk testManualChunk
			_ = ro.Unpack(&chunk)
		}
	})

	b.Run("unpack/reflect", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			var chunk testReflectChunk
			_ = ro.Unpack(&chunk)
		}
	})

	b.Run("unpack/manual_get", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = GetAs[string](ro, "sentence", "")
			_ = GetAs[bool](ro, "is_final", false)
			_ = GetAs[string](ro, "emotion", "")
		}
	})
}
