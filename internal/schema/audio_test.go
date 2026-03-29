package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAudio_Lifecycle(t *testing.T) {
	initialCount := LoadActiveAudioCount()

	t.Run("New and Release", func(t *testing.T) {
		audio := NewAudio("test.wav", 16000, 1)
		assert.Equal(t, initialCount+1, LoadActiveAudioCount())
		assert.Equal(t, "test.wav", audio.Name())

		audio.Release()
		assert.Equal(t, initialCount, LoadActiveAudioCount())
	})

	t.Run("Reference Counting", func(t *testing.T) {
		audio := NewAudio("ref.wav", 16000, 1)
		audio.Retain()
		assert.Equal(t, initialCount+1, LoadActiveAudioCount())

		audio.Release()
		assert.Equal(t, initialCount+1, LoadActiveAudioCount(), "Should not recycle yet")

		audio.Release()
		assert.Equal(t, initialCount, LoadActiveAudioCount(), "Should recycle now")
	})

	t.Run("Count with Mutable Clone", func(t *testing.T) {
		initial := LoadActiveAudioCount()
		audio := NewAudio("cow.wav", 16000, 1)
		ro := audio.ReadOnly()
		assert.Equal(t, initial+1, LoadActiveAudioCount())

		mutable := ro.Mutable()
		assert.NotSame(t, ro, mutable)
		assert.Equal(t, initial+2, LoadActiveAudioCount(), "Mutable clone should create a new active object")

		mutable.Release()
		assert.Equal(t, initial+1, LoadActiveAudioCount())

		ro.Release()
		assert.Equal(t, initial, LoadActiveAudioCount())
	})
}

func TestAudio_Buffer(t *testing.T) {
	t.Run("Set and Get Bytes", func(t *testing.T) {
		audio := NewAudio("buff.wav", 16000, 1)
		defer audio.Release()

		data := []byte("audio-data")
		audio.SetBytes(data)
		assert.Equal(t, data, audio.Bytes())

		// Verify deep copy
		data[0] = 'X'
		assert.NotEqual(t, data, audio.Bytes())
	})

	t.Run("Buffer Reuse", func(t *testing.T) {
		audio := NewAudio("reuse.wav", 16000, 1)
		defer audio.Release()

		// Set small buffer
		audio.SetBytes([]byte("small"))
		oldCap := cap(audio.Bytes())

		// Set another buffer that fits in cap
		audio.SetBytes([]byte("hi"))
		assert.Equal(t, oldCap, cap(audio.Bytes()))
	})
}

func TestAudio_COW(t *testing.T) {
	t.Run("Mutable", func(t *testing.T) {
		audio := NewAudio("cow1.wav", 16000, 1)
		defer audio.Release()

		imm := audio.ReadOnly()
		assert.Equal(t, audio.Bytes(), imm.Bytes())

		// Should be same physical object initially
		inner := imm.(*builtinAudio)
		assert.True(t, inner.readonly.Load())
	})

	t.Run("Mutable - Same Object", func(t *testing.T) {
		audio := NewAudio("cow2.wav", 16000, 1)
		defer audio.Release()

		writable := audio.(*builtinAudio).Mutable()
		assert.Equal(t, audio, writable)
	})

	t.Run("Mutable - New Object Copy", func(t *testing.T) {
		audio := NewAudio("cow3.wav", 16000, 1)
		defer audio.Release()
		_ = audio.Set("key1", "val1")
		audio.SetBytes([]byte("original"))

		imm := audio.ReadOnly()

		// Since it's readonly, Mutable should return a NEW object
		writable := imm.Mutable()
		defer writable.Release()

		assert.NotEqual(t, imm, writable)
		assert.Equal(t, "cow3.wav", writable.Name())
		assert.Equal(t, []byte("original"), writable.Bytes())

		val, _ := writable.Get("key1")
		assert.Equal(t, "val1", val)

		// Modification on new object shouldn't affect old one
		_ = writable.Set("key1", "val2")
		valImm, _ := imm.Get("key1")
		assert.Equal(t, "val1", valImm)
	})
}

func TestAudio_Recycle(t *testing.T) {
	t.Run("Recycle Cap Guards", func(t *testing.T) {
		audio := NewAudio("recycle.wav", 16000, 1).(*builtinAudio)

		// Fill with large data/entries
		audio.buffer = make([]byte, audioRecycleCap+1)
		audio.entries = make([]entry, entriesRecycleCap+1)

		audio.Recycle()

		assert.Nil(t, audio.buffer, "Large buffer should be nilled")
		assert.Empty(t, audio.entries, "Large entries should be fresh slice")
		assert.Equal(t, 0, cap(audio.entries))
	})

	t.Run("Small Recycle Reuse", func(t *testing.T) {
		audio := NewAudio("reuse.wav", 16000, 1).(*builtinAudio)
		audio.buffer = make([]byte, audioRecycleCap-1)
		audio.entries = make([]entry, entriesRecycleCap-1)
		oldEntryCap := cap(audio.entries)

		audio.Recycle()

		assert.NotNil(t, audio.buffer)
		assert.Empty(t, audio.buffer)
		assert.Equal(t, oldEntryCap, cap(audio.entries), "Small entries should be resliced, not remade")
	})
}
