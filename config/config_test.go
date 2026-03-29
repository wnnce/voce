package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterConfigureReaders(t *testing.T) {
	mutex.Lock()
	readerMap = make(map[uintptr]bool)
	readerSlice = nil
	mutex.Unlock()

	fn := func(ctx context.Context) (func(), error) { return nil, nil }
	RegisterConfigureReaders(fn, fn)

	assert.Len(t, readerSlice, 1)
}

func TestDoReaderConfiguration(t *testing.T) {
	t.Run("success with cleanup", func(t *testing.T) {
		mutex.Lock()
		readerMap = make(map[uintptr]bool)
		readerSlice = nil
		mutex.Unlock()

		var cleaned bool
		fn := func(ctx context.Context) (func(), error) {
			return func() { cleaned = true }, nil
		}
		RegisterConfigureReaders(fn)
		cleanup, err := DoReaderConfiguration(context.Background())
		require.NoError(t, err)
		cleanup()
		assert.True(t, cleaned)
	})

	t.Run("error and rollback", func(t *testing.T) {
		mutex.Lock()
		readerMap = make(map[uintptr]bool)
		readerSlice = nil
		mutex.Unlock()

		var aCleaned bool
		fnA := func(ctx context.Context) (func(), error) {
			return func() { aCleaned = true }, nil
		}
		fnB := func(ctx context.Context) (func(), error) {
			return nil, errors.New("failed")
		}

		RegisterConfigureReaders(fnA, fnB)

		cleanup, err := DoReaderConfiguration(context.Background())
		require.Error(t, err)
		assert.Nil(t, cleanup)
		assert.True(t, aCleaned)
	})
}

func TestViperGet(t *testing.T) {
	viper.Reset()
	viper.Set("foo", "bar")
	viper.Set("port", 8080)
	viper.Set("enabled", true)
	viper.Set("duration", "10s")
	viper.Set("float", 1.23)

	t.Run("string exists", func(t *testing.T) {
		assert.Equal(t, "bar", ViperGet[string]("foo"))
	})

	t.Run("bool exists", func(t *testing.T) {
		assert.True(t, ViperGet[bool]("enabled"))
	})

	t.Run("float exists", func(t *testing.T) {
		assert.InDelta(t, 1.23, ViperGet[float64]("float"), 1e-9)
	})

	t.Run("duration exists", func(t *testing.T) {
		assert.Equal(t, 10*time.Second, ViperGet[time.Duration]("duration"))
	})

	t.Run("default value", func(t *testing.T) {
		assert.Equal(t, "fallback", ViperGet[string]("notfound", "fallback"))
	})
}
