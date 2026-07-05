package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultBuilderBuildFreezesResult(t *testing.T) {
	builder := NewResultBuilder("tool_call", ResultStatusOK)
	require.NoError(t, builder.Set("message", "done"))
	require.NoError(t, builder.Set("count", 3))

	result := builder.Build()

	t.Run("result exposes readonly fields", func(t *testing.T) {
		assert.Equal(t, "tool_call", result.Name())
		assert.Equal(t, ResultStatusOK, result.Status())
		assert.Equal(t, "done", GetAs[string](result, "message"))
		assert.Equal(t, 3, GetAs[int](result, "count"))
	})

	t.Run("builder rejects writes after build", func(t *testing.T) {
		err := builder.Set("late", true)
		require.ErrorIs(t, err, ErrReadOnly)
		assert.Empty(t, GetAs[bool](result, "late"))
	})
}

func TestResultBuilderPackAndUnpack(t *testing.T) {
	type payload struct {
		Text string `schema:"text"`
		Size int    `schema:"size"`
	}

	builder := NewResultBuilder("summary", ResultStatusCanceled)
	require.NoError(t, builder.Pack(payload{
		Text: "hello",
		Size: 5,
	}))

	result := builder.Build()

	t.Run("unpack returns packed fields", func(t *testing.T) {
		var decoded payload
		require.NoError(t, result.Unpack(&decoded))
		assert.Equal(t, payload{
			Text: "hello",
			Size: 5,
		}, decoded)
	})

	t.Run("result preserves status", func(t *testing.T) {
		assert.Equal(t, ResultStatusCanceled, result.Status())
	})
}

func TestResultBuildIsIdempotent(t *testing.T) {
	builder := NewResultBuilder("noop", ResultStatusUnspecified)

	first := builder.Build()
	second := builder.Build()

	t.Run("build returns same immutable instance", func(t *testing.T) {
		assert.Same(t, first, second)
	})

	t.Run("result keeps unspecified status", func(t *testing.T) {
		assert.Equal(t, ResultStatusUnspecified, first.Status())
	})
}
