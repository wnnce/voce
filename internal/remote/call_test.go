package remote

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
)

func TestCall(t *testing.T) {
	t.Run("FinishSuccessfully", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-1", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 1*time.Second)

		// Finish immediately
		c.finish(nil)

		err := c.wait()
		assert.NoError(t, err)
	})

	t.Run("FinishWithError", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-2", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 1*time.Second)

		expectedErr := errors.New("something went wrong")
		c.finish(expectedErr)

		err := c.wait()
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("WaitTimeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := context.Background()
			// short lease for fast timeout
			c := newCall(ctx, "id-timeout", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 50*time.Millisecond)

			err := c.wait()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "remote call lease expired")
		})
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := newCall(ctx, "id-cancel", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 1*time.Second)

		// Cancel context immediately
		cancel()

		err := c.wait()
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("RenewExtendsTimeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := context.Background()
			c := newCall(ctx, "id-renew", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 100*time.Millisecond)

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
		c := newCall(ctx, "id-idem", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 1*time.Second)

		err1 := errors.New("first error")
		err2 := errors.New("second error")

		c.finish(err1)
		c.finish(err2) // Should be ignored

		err := c.wait()
		assert.ErrorIs(t, err, err1)
	})

	t.Run("RenewAfterFinish", func(t *testing.T) {
		ctx := context.Background()
		c := newCall(ctx, "id-idem", pluginv1.RuntimeMessageType_RUNTIME_MESSAGE_TYPE_SIGNAL, "test", "inst", 1*time.Second)

		c.finish(nil)

		// Should not panic or hang
		c.renew()
		c.renew()

		err := c.wait()
		assert.NoError(t, err)
	})
}
