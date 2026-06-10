package retry

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackoff_TryAndFail(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBackoff(100*time.Millisecond, 500*time.Millisecond)

		// Initially should be allowed
		assert.True(t, b.Try())

		// Record failure, backoff = 100ms
		b.Fail()

		// Immediately after fail, should be blocked
		assert.False(t, b.Try())

		// Advance past the 100ms window
		time.Sleep(100 * time.Millisecond)
		assert.True(t, b.Try())

		// Fail again, backoff advances to 200ms
		b.Fail()
		time.Sleep(100 * time.Millisecond)
		assert.False(t, b.Try(), "should still be blocked, backoff is now 200ms")

		time.Sleep(100 * time.Millisecond)
		assert.True(t, b.Try())
	})
}

func TestBackoff_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBackoff(100*time.Millisecond, 500*time.Millisecond)

		// Fail twice to advance backoff to 200ms
		b.Fail()
		time.Sleep(100 * time.Millisecond)
		b.Fail()

		// Reset should restore to initial
		b.Reset()
		assert.True(t, b.Try())

		// After reset + fail, backoff should be back to 100ms (not 200ms)
		b.Fail()
		time.Sleep(100 * time.Millisecond)
		assert.True(t, b.Try())
	})
}

func TestBackoff_Wait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBackoff(50*time.Millisecond, 200*time.Millisecond)
		ctx := context.Background()

		// First wait = 50ms
		start := time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 50*time.Millisecond, time.Since(start))

		// Second wait = 100ms (50 * 2)
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 100*time.Millisecond, time.Since(start))

		// Third wait = 200ms (capped at max)
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 200*time.Millisecond, time.Since(start))

		// Fourth wait = still 200ms (stays at max)
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 200*time.Millisecond, time.Since(start))

		// Reset brings it back to 50ms
		b.Reset()
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 50*time.Millisecond, time.Since(start))
	})
}

func TestBackoff_WaitContextCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBackoff(500*time.Millisecond, 1*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := b.Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 50*time.Millisecond, time.Since(start))
	})
}

func TestBackoff_CustomMultiplier(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBackoff(100*time.Millisecond, 1*time.Second, 3.0)
		ctx := context.Background()

		// 100ms
		start := time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 100*time.Millisecond, time.Since(start))

		// 300ms (100 * 3)
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 300*time.Millisecond, time.Since(start))

		// 900ms (300 * 3)
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 900*time.Millisecond, time.Since(start))

		// 1s (capped at max, 2700ms -> 1000ms)
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 1*time.Second, time.Since(start))
	})
}

func TestBackoff_InvalidMultiplier(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// multiplier <= 1.0 should fall back to default 2.0
		b := NewBackoff(50*time.Millisecond, 200*time.Millisecond, 0.5)
		ctx := context.Background()

		start := time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 50*time.Millisecond, time.Since(start))

		// Should be 100ms (50 * 2.0), proving default multiplier is used
		start = time.Now()
		require.NoError(t, b.Wait(ctx))
		assert.Equal(t, 100*time.Millisecond, time.Since(start))
	})
}
