package machine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
)

func TestConnectionManagerSelect(t *testing.T) {
	t.Run("returns nil without active connections", func(t *testing.T) {
		m := newTestConnectionManager()
		assert.Nil(t, m.Select(testSessionKey(1)))
		assert.Nil(t, m.Lookup(testSessionKey(1)))
	})

	t.Run("keeps each session on its original connection", func(t *testing.T) {
		m := newTestConnectionManager()
		first := activeTestConnection()
		second := activeTestConnection()
		m.Store(first)
		m.Store(second)

		firstKey := testSessionKey(1)
		secondKey := testSessionKey(2)
		require.Same(t, first, m.Select(firstKey))
		require.Same(t, first, m.Select(firstKey))
		require.Same(t, second, m.Select(secondKey))

		assert.Same(t, first, m.Lookup(firstKey))
		assert.Equal(t, 1, m.connections[first].load)
		assert.Equal(t, 1, m.connections[second].load)
	})

	t.Run("new connections do not migrate existing routes", func(t *testing.T) {
		m := newTestConnectionManager()
		first := activeTestConnection()
		second := activeTestConnection()
		third := activeTestConnection()
		m.Store(first)
		m.Store(second)

		firstKey := testSessionKey(1)
		secondKey := testSessionKey(2)
		require.Same(t, first, m.Select(firstKey))
		require.Same(t, second, m.Select(secondKey))

		m.Store(third)
		assert.Same(t, first, m.Select(firstKey))
		assert.Same(t, second, m.Select(secondKey))
		assert.Same(t, third, m.Select(testSessionKey(3)))
	})

	t.Run("skips inactive connections", func(t *testing.T) {
		m := newTestConnectionManager()
		inactive := activeTestConnection()
		active := activeTestConnection()
		m.Store(inactive)
		m.Store(active)
		inactive.state.Store(int32(protocol.ConnectionClosed))

		assert.Same(t, active, m.Select(testSessionKey(1)))
		_, exists := m.connections[inactive]
		assert.False(t, exists)
	})
}

func TestConnectionManagerReleaseAndRemove(t *testing.T) {
	t.Run("release makes the session route available for reassignment", func(t *testing.T) {
		m := newTestConnectionManager()
		first := activeTestConnection()
		second := activeTestConnection()
		m.Store(first)
		m.Store(second)

		firstKey := testSessionKey(1)
		secondKey := testSessionKey(2)
		require.Same(t, first, m.Select(firstKey))
		require.Same(t, second, m.Select(secondKey))

		m.Release(firstKey)
		m.Release(firstKey)
		assert.Nil(t, m.Lookup(firstKey))
		assert.Zero(t, m.connections[first].load)
		assert.Same(t, first, m.Select(testSessionKey(3)))
	})

	t.Run("remove clears only its assigned routes", func(t *testing.T) {
		m := newTestConnectionManager()
		first := activeTestConnection()
		second := activeTestConnection()
		m.Store(first)
		m.Store(second)

		firstKey := testSessionKey(1)
		secondKey := testSessionKey(2)
		require.Same(t, first, m.Select(firstKey))
		require.Same(t, second, m.Select(secondKey))

		m.Remove(first)
		assert.Nil(t, m.Lookup(firstKey))
		assert.Same(t, second, m.Lookup(secondKey))
		_, exists := m.connections[first]
		assert.False(t, exists)
		assert.Len(t, m.active, 1)
		assert.Same(t, second, m.Select(firstKey))
	})
}

func activeTestConnection() *Connection {
	conn := NewConnection(nil)
	conn.state.Store(int32(protocol.ConnectionActive))
	return conn
}

func newTestConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[*Connection]*heapConnection),
		routes:      make(map[protocol.SessionKey]*heapConnection),
	}
}

func testSessionKey(value byte) protocol.SessionKey {
	return protocol.SessionKey{value}
}
