package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
)

func TestDynamicConnectionPoolBind(t *testing.T) {
	t.Run("keeps the existing binding", func(t *testing.T) {
		p := newTestConnectionPool(2, 0)
		defer p.Shutdown()

		key := testSessionKey(1)
		first := p.Bind(key)
		second := p.Bind(key)

		require.Same(t, first, second)
		assert.NotNil(t, first.Connection())
		assert.Len(t, p.connections, 1)
		assert.Equal(t, 1, p.connections[first.Connection()].load)
	})

	t.Run("adds a connection after reaching per connection capacity", func(t *testing.T) {
		p := newTestConnectionPool(1, 0)
		defer p.Shutdown()

		first := p.Bind(testSessionKey(1))
		second := p.Bind(testSessionKey(2))

		require.Eventually(t, func() bool {
			return second.Connection() != nil
		}, time.Second, time.Millisecond)
		assert.NotSame(t, first.Connection(), second.Connection())
		assert.Equal(t, 2, connectionCount(p))
	})

	t.Run("leaves the binding empty when the connection limit is reached", func(t *testing.T) {
		p := newTestConnectionPool(1, 1)
		defer p.Shutdown()

		firstKey := testSessionKey(1)
		first := p.Bind(firstKey)
		second := p.Bind(testSessionKey(2))
		assert.NotNil(t, first.Connection())
		assert.Nil(t, second.Connection())
		assert.Len(t, p.connections, 1)

		p.Unbind(firstKey)
		assert.NotNil(t, second.Connection())
	})
}

func TestDynamicConnectionPoolScalesAtTarget(t *testing.T) {
	p := newTestConnectionPool(64, 0)
	defer p.Shutdown()
	p.targetSessions = 1

	first := p.Bind(testSessionKey(1))
	require.NotNil(t, first.Connection())
	require.Eventually(t, func() bool {
		return connectionCount(p) == 2
	}, time.Second, time.Millisecond)

	second := p.Bind(testSessionKey(2))
	assert.NotSame(t, first.Connection(), second.Connection())
}

func TestDynamicConnectionPoolReusesReleasedCapacity(t *testing.T) {
	p := newTestConnectionPool(64, 0)
	defer p.Shutdown()
	addTestConnection(p)

	firstKey := testSessionKey(1)
	first := p.Bind(firstKey)
	second := p.Bind(testSessionKey(2))
	require.NotSame(t, first.Connection(), second.Connection())

	p.Unbind(firstKey)
	third := p.Bind(testSessionKey(3))
	assert.Same(t, first.Connection(), third.Connection())
}

func TestDynamicConnectionPoolUnbindAndCleanup(t *testing.T) {
	p := newTestConnectionPool(1, 0)
	defer p.Shutdown()

	key := testSessionKey(1)
	binding := p.Bind(key)
	conn := binding.Connection()
	require.NotNil(t, conn)

	p.Unbind(key)
	assert.Zero(t, p.connections[conn].load)

	p.mu.Lock()
	p.connections[conn].idleSince = time.Now().Add(-time.Second)
	p.mu.Unlock()
	p.cleanupIdle(time.Now())

	assert.Empty(t, p.connections)
	assert.Equal(t, protocol.ConnectionClosed, conn.State())
}

func TestDynamicConnectionPoolKeepsMinimumConnections(t *testing.T) {
	p := newTestConnectionPool(1, 0)
	p.minConns = 1
	defer p.Shutdown()

	key := testSessionKey(1)
	conn := p.Bind(key).Connection()
	require.NotNil(t, conn)
	p.Unbind(key)

	p.mu.Lock()
	p.connections[conn].idleSince = time.Now().Add(-time.Second)
	p.mu.Unlock()
	p.cleanupIdle(time.Now())

	assert.Equal(t, 1, connectionCount(p))
	assert.Equal(t, protocol.ConnectionActive, conn.State())
}

func TestDynamicConnectionPoolDropsClosedConnections(t *testing.T) {
	p := newTestConnectionPool(2, 0)
	defer p.Shutdown()

	first := p.Bind(testSessionKey(1))
	closed := first.Connection()
	require.NotNil(t, closed)
	closed.state.Store(int32(protocol.ConnectionClosed))

	second := p.Bind(testSessionKey(2))
	require.Eventually(t, func() bool {
		return second.Connection() != nil
	}, time.Second, time.Millisecond)
	assert.Same(t, second.Connection(), first.Connection())
	assert.NotSame(t, closed, second.Connection())
	assert.NotContains(t, p.connections, closed)
}

func TestNewConnectionPoolRejectsNilEngine(t *testing.T) {
	pool, err := NewConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{}, nil)
	assert.Nil(t, pool)
	assert.ErrorIs(t, err, ErrNilNBHTTPEngine)
}

func newTestConnectionPool(maxSessions, maxConns int) *ConnectionPool {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		MaxSessionsPerConnection: maxSessions,
		MaxConnections:           maxConns,
		IdleTimeout:              time.Millisecond,
		CleanupInterval:          time.Hour,
	}, nil)
	p.newConnection = func() (*Connection, error) {
		return testConnection(), nil
	}
	p.connect = func(*Connection) error { return nil }
	addTestConnection(p)
	return p
}

func addTestConnection(p *ConnectionPool) {
	p.mu.Lock()
	p.pendingDials++
	p.mu.Unlock()
	p.startDial()
}

func connectionCount(p *ConnectionPool) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.connections)
}

func testConnection() *Connection {
	conn := &Connection{}
	conn.state.Store(int32(protocol.ConnectionActive))
	return conn
}

func testSessionKey(value byte) protocol.SessionKey {
	return protocol.SessionKey{value}
}

func TestDynamicConnectionPoolConnectionFactoryFailure(t *testing.T) {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		MaxSessionsPerConnection: 1,
		CleanupInterval:          time.Hour,
	}, nil)
	defer p.Shutdown()
	p.newConnection = func() (*Connection, error) {
		return nil, errors.New("dial setup failed")
	}

	binding := p.Bind(testSessionKey(1))
	assert.Nil(t, binding.Connection())
	require.Eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.pendingDials == 0
	}, time.Second, time.Millisecond)
	assert.Empty(t, p.connections)
}
