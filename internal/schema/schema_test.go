package schema

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinProperties_Get(t *testing.T) {
	props := &builtinProperties{
		entries: []entry{
			{key: "foo", val: "bar"},
		},
	}

	t.Run("key exists", func(t *testing.T) {
		val, ok := props.Get("foo")
		assert.True(t, ok)
		assert.Equal(t, "bar", val)
	})

	t.Run("key not exists", func(t *testing.T) {
		val, ok := props.Get("notfound")
		assert.False(t, ok)
		assert.Nil(t, val)
	})
}

func TestBuiltinProperties_Set(t *testing.T) {
	props := &builtinProperties{}

	t.Run("add new key", func(t *testing.T) {
		require.NoError(t, props.Set("key1", "val1"))
		val, ok := props.Get("key1")
		assert.True(t, ok)
		assert.Equal(t, "val1", val)
	})

	t.Run("overwrite existing key", func(t *testing.T) {
		require.NoError(t, props.Set("key1", "val2"))
		val, ok := props.Get("key1")
		assert.True(t, ok)
		assert.Equal(t, "val2", val)
	})

	t.Run("safeValue types", func(t *testing.T) {
		types := []struct {
			name     string
			input    any
			expected any
		}{
			{"int", 123, 123},
			{"bool", true, true},
			{"string", "hello", "hello"},
			{"duration", 5 * time.Second, 5 * time.Second},
			{"bytes", []byte("world"), []byte("world")},
			{"bytesBuffer", bytes.NewBufferString("buf"), []byte("buf")},
			{"stringsBuilderPtr", func() any {
				var b strings.Builder
				b.WriteString("builder")
				return &b
			}(), "builder"},
			{"customStruct", struct{ Name string }{Name: "voce"}, []byte(`{"Name":"voce"}`)},
		}

		for _, tc := range types {
			t.Run(tc.name, func(t *testing.T) {
				key := "type_" + tc.name
				require.NoError(t, props.Set(key, tc.input))
				val, _ := props.Get(key)

				assert.Equal(t, tc.expected, val)
				// Check for cloning
				if tc.name == "bytes" {
					inputBytes := tc.input.([]byte)
					inputBytes[0] = 'X'
					assert.NotEqual(t, inputBytes, val)
				}
			})
		}
	})
}

func TestBuiltinProperties_Bind(t *testing.T) {
	props := &builtinProperties{}

	t.Run("key not found", func(t *testing.T) {
		var dst string
		assert.ErrorIs(t, props.Bind("unknown", &dst), ErrKeyNotFound)
	})

	t.Run("not serialized bytes", func(t *testing.T) {
		_ = props.Set("not_bytes", 123)
		var dst string
		assert.ErrorIs(t, props.Bind("not_bytes", &dst), ErrNotSerializedBytes)
	})

	t.Run("success bind", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
		}
		_ = props.Set("user", Person{Name: "alice"})

		var res Person
		require.NoError(t, props.Bind("user", &res))
		assert.Equal(t, "alice", res.Name)
	})
}

func TestBuiltinProperties_Clone(t *testing.T) {
	props := &builtinProperties{}
	_ = props.Set("a", 1)

	clone := props.Clone()
	assert.Equal(t, props.entries, clone.entries)

	_ = props.Set("b", 2)
	_, ok := clone.Get("b")
	assert.False(t, ok)
}

func TestGetAs(t *testing.T) {
	props := &builtinProperties{}
	_ = props.Set("str", "hello")
	_ = props.Set("num", 123)

	t.Run("success match", func(t *testing.T) {
		assert.Equal(t, "hello", GetAs[string](props, "str"))
		assert.Equal(t, 123, GetAs[int](props, "num"))
	})

	t.Run("type mismatch with default", func(t *testing.T) {
		assert.Equal(t, "def", GetAs[string](props, "num", "def"))
	})

	t.Run("type mismatch with zero", func(t *testing.T) {
		assert.Empty(t, GetAs[string](props, "num"))
	})

	t.Run("key not found with default", func(t *testing.T) {
		assert.Equal(t, 999, GetAs[int](props, "notfound", 999))
	})

	t.Run("key not found with zero", func(t *testing.T) {
		assert.Equal(t, 0, GetAs[int](props, "notfound"))
	})
}

func TestBuiltinPayload_ReadOnlyEnforcement(t *testing.T) {
	payload := NewPayload("test")
	_ = payload.Set("a", 1)

	ro := payload.ReadOnly()
	t.Run("Set should fail", func(t *testing.T) {
		err := ro.(Properties).Set("b", 2)
		assert.ErrorIs(t, err, ErrReadOnly)
	})

	t.Run("Mutable should return a new object", func(t *testing.T) {
		mutable := ro.Mutable()
		assert.NotSame(t, ro, mutable)
		assert.NoError(t, mutable.Set("b", 2))
	})

	t.Run("Direct type cast and Set should fail", func(t *testing.T) {
		inner := ro.(*builtinPayload)
		err := inner.Set("c", 3)
		assert.ErrorIs(t, err, ErrReadOnly)
	})
}

func TestBuiltinAudio_ReadOnlyEnforcement(t *testing.T) {
	audio := NewAudio("test", 16000, 1)
	ro := audio.ReadOnly()

	t.Run("Direct type cast and SetBytes should panic", func(t *testing.T) {
		inner := ro.(*builtinAudio)
		assert.Panics(t, func() {
			inner.SetBytes([]byte{1, 2, 3})
		})
	})

	t.Run("SetSampleRate should panic", func(t *testing.T) {
		assert.Panics(t, func() {
			audio.SetSampleRate(44100)
		})
	})

	t.Run("Set should fail", func(t *testing.T) {
		err := audio.Set("k", "v")
		assert.ErrorIs(t, err, ErrReadOnly)
	})

	t.Run("Mutable should allow modification", func(t *testing.T) {
		mutable := ro.Mutable()
		assert.NotSame(t, ro, mutable)
		assert.NotPanics(t, func() {
			mutable.SetBytes([]byte{1, 2, 3})
		})
	})
}

func TestBuiltinProperties_CopyTo(t *testing.T) {
	src := &builtinProperties{}
	_ = src.Set("k1", "v1")
	_ = src.Set("k2", "v2")

	t.Run("copy to empty", func(t *testing.T) {
		dst := &builtinProperties{}
		src.copyTo(dst)
		assert.Equal(t, src.entries, dst.entries)
		assert.Len(t, dst.entries, 2)
	})

	t.Run("reuse capacity", func(t *testing.T) {
		// Create dst with enough capacity
		dst := &builtinProperties{
			entries: make([]entry, 0, 10),
		}
		// Get a pointer to the underlying array to verify reuse
		initialSlice := dst.entries[0:10]
		src.copyTo(dst)
		assert.Equal(t, src.entries, dst.entries)
		assert.Equal(t, &initialSlice[0], &dst.entries[0:1][0], "should reuse same underlying array")
	})

	t.Run("expand capacity", func(t *testing.T) {
		// Create dst with insufficient capacity
		dst := &builtinProperties{
			entries: make([]entry, 0, 1),
		}
		src.copyTo(dst)
		assert.Equal(t, src.entries, dst.entries)
		assert.GreaterOrEqual(t, cap(dst.entries), 2)
	})

	t.Run("independence", func(t *testing.T) {
		dst := &builtinProperties{}
		src.copyTo(dst)
		_ = src.Set("k3", "v3")

		_, ok := dst.Get("k3")
		assert.False(t, ok, "dst should not be affected by changes in src")
	})
}
