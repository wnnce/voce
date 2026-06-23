package remote

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCall(t *testing.T) {
	t.Run("FinishSuccessfully", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-1", 1*time.Second)

		// Finish immediately
		c.finish(nil)

		err := c.wait()
		assert.NoError(t, err)
	})

	t.Run("FinishWithError", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-2", 1*time.Second)

		expectedErr := errors.New("something went wrong")
		c.finish(expectedErr)

		err := c.wait()
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("WaitTimeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := context.Background()
			// short lease for fast timeout
			c := newCall(ctx, "id-timeout", 50*time.Millisecond)

			err := c.wait()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "remote call lease expired")
		})
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := newCall(ctx, "id-cancel", 1*time.Second)

		// Cancel context immediately
		cancel()

		err := c.wait()
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("ContextCancellationCallbackFalseWaitsForFinish", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		callbackCalled := make(chan struct{}, 1)
		c := newCall(ctx, "id-cancel-wait", time.Second, withCancelCallback(func() bool {
			callbackCalled <- struct{}{}
			return false
		}))

		done := make(chan error, 1)
		go func() {
			done <- c.wait()
		}()

		cancel()
		require.Eventually(t, func() bool {
			select {
			case <-callbackCalled:
				return true
			default:
				return false
			}
		}, time.Second, 10*time.Millisecond)

		select {
		case err := <-done:
			t.Fatalf("wait returned before finish: %v", err)
		default:
		}

		c.finish(nil)
		require.NoError(t, <-done)
	})

	t.Run("ContextCancellationCallbackTrueReturnsCanceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		callbackCalled := false
		c := newCall(ctx, "id-cancel-return", time.Second, withCancelCallback(func() bool {
			callbackCalled = true
			return true
		}))

		cancel()

		err := c.wait()
		assert.True(t, callbackCalled)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("LeaseExpirationInvokesExpireCallback", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := context.Background()
			expired := false
			c := newCall(ctx, "id-expire-callback", 50*time.Millisecond, withExpireCallback(func() {
				expired = true
			}))

			err := c.wait()

			require.Error(t, err)
			assert.True(t, expired)
			assert.Contains(t, err.Error(), "remote call lease expired")
		})
	})

	t.Run("RenewExtendsTimeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := context.Background()
			c := newCall(ctx, "id-renew", 100*time.Millisecond)

			// In a background goroutine, renew a few times, then finish
			go func() {
				time.Sleep(60 * time.Millisecond)
				c.renew()
				time.Sleep(60 * time.Millisecond)
				c.renew()
				time.Sleep(60 * time.Millisecond)
				c.finish(nil)
			}()

			err := c.wait()
			assert.NoError(t, err)
		})
	})

	t.Run("FinishIdempotent", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-idem", 1*time.Second)

		err1 := errors.New("first error")
		err2 := errors.New("second error")

		c.finish(err1)
		c.finish(err2) // Should be ignored

		err := c.wait()
		assert.ErrorIs(t, err, err1)
	})

	t.Run("RenewAfterFinish", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-idem", 1*time.Second)

		c.finish(nil)

		// Should not panic or hang
		c.renew()
		c.renew()

		err := c.wait()
		assert.NoError(t, err)
	})
}
