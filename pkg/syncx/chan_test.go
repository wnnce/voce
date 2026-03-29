package syncx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendWithContext(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ch := make(chan int, 1)
		err := SendWithContext(context.Background(), ch, 42)
		require.NoError(t, err)
		assert.Equal(t, 42, <-ch)
	})

	t.Run("context_cancel", func(t *testing.T) {
		ch := make(chan int) // Unbuffered, will block
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := SendWithContext(ctx, ch, 42)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("pre_canceled", func(t *testing.T) {
		ch := make(chan int, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := SendWithContext(ctx, ch, 42)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestSendWithTimeout(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		ch := make(chan int) // Will block
		err := SendWithTimeout(context.Background(), ch, 42, 10*time.Millisecond)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("success", func(t *testing.T) {
		ch := make(chan int, 1)
		err := SendWithTimeout(context.Background(), ch, 42, 100*time.Millisecond)
		assert.NoError(t, err)
	})
}

func TestSendWithNonBlocking(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ch := make(chan int, 1)
		err := SendWithNonBlocking(context.Background(), ch, 42)
		assert.NoError(t, err)
	})

	t.Run("blocked", func(t *testing.T) {
		ch := make(chan int) // Unbuffered, no receiver
		err := SendWithNonBlocking(context.Background(), ch, 42)
		assert.ErrorIs(t, err, ErrSendBlocked)
	})

	t.Run("full_buffered", func(t *testing.T) {
		ch := make(chan int, 1)
		ch <- 1
		err := SendWithNonBlocking(context.Background(), ch, 42)
		assert.ErrorIs(t, err, ErrSendBlocked)
	})
}
