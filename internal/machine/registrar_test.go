package machine

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrar(t *testing.T) {
	ctx := context.Background()

	t.Run("uses configured ID", func(t *testing.T) {
		registrar := NewRegistrar(ctx, "machine-a", "gateway:8080", 9000)
		assert.Equal(t, "machine-a", registrar.id)
		assert.Equal(t, ctx, registrar.ctx)
		require.NotNil(t, registrar.backoff)
	})

	t.Run("generates ID when omitted", func(t *testing.T) {
		registrar := NewRegistrar(ctx, "", "gateway:8080", 9000)
		assert.NotEmpty(t, registrar.id)
	})
}

func TestRegistrarRegistrationURL(t *testing.T) {
	registrar := NewRegistrar(context.Background(), "machine a", "gateway:8080", 9000)
	u := registrar.registrationURL()

	assert.Equal(t, "ws", u.Scheme)
	assert.Equal(t, "gateway:8080", u.Host)
	assert.Equal(t, "/register", u.Path)
	values, err := url.ParseQuery(u.RawQuery)
	require.NoError(t, err)
	assert.Equal(t, "machine a", values.Get("id"))
	assert.Equal(t, "9000", values.Get("port"))
}

func TestRegistrarReconnectLoopStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	registrar := NewRegistrar(ctx, "machine-a", "gateway:8080", 9000)
	done := make(chan struct{})
	go func() {
		registrar.reconnectLoop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconnect loop did not stop after context cancellation")
	}
}
