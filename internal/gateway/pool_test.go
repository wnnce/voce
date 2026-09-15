package gateway

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

func TestDynamicConnectionPoolBindSameKeyConcurrently(t *testing.T) {
	p := newTestConnectionPool(64, 0)
	defer p.Shutdown()

	key := testSessionKey(1)
	const workers = 32
	start := make(chan struct{})
	bindings := make(chan *SessionBinding, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			bindings <- p.Bind(key)
		}()
	}
	close(start)
	wg.Wait()
	close(bindings)

	var binding *SessionBinding
	for current := range bindings {
		if binding == nil {
			binding = current
			continue
		}
		assert.Same(t, binding, current)
	}
	require.NotNil(t, binding)
	p.mu.Lock()
	assert.Equal(t, 1, p.connections[binding.Connection()].load)
	p.mu.Unlock()
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

func TestDynamicConnectionPoolPrioritizesAllocatableConnections(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()

	firstKey := testSessionKey(1)
	first := p.Bind(firstKey).Connection()
	second := addTestConnectionWithState(p, protocol.ConnectionActive)
	p.Bind(testSessionKey(2))
	p.Unbind(firstKey)

	first.state.Store(int32(protocol.ConnectionConnecting))
	p.OnConnectionClose(first)
	assert.Same(t, second, p.Bind(testSessionKey(3)).Connection())

	first.state.Store(int32(protocol.ConnectionActive))
	p.OnConnectionOpen(first)
	assert.Same(t, first, p.Bind(testSessionKey(4)).Connection())
}

func TestDynamicConnectionPoolUsesConnectingCapacityAfterActiveIsFull(t *testing.T) {
	p := newTestConnectionPool(1, 0)
	defer p.Shutdown()

	active := p.Bind(testSessionKey(1)).Connection()
	connecting := addTestConnectionWithState(p, protocol.ConnectionConnecting)

	assert.Same(t, connecting, p.Bind(testSessionKey(2)).Connection())
	assert.Nil(t, p.Bind(testSessionKey(3)).Connection())
	assert.Equal(t, connectionPriorityUnavailable, p.connections[active].priority)
	assert.Equal(t, connectionPriorityUnavailable, p.connections[connecting].priority)
}

func TestDynamicConnectionPoolTracksConnectionLifecycle(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()

	active := p.Bind(testSessionKey(1)).Connection()
	connecting := addTestConnectionWithState(p, protocol.ConnectionConnecting)
	connecting.OnOpen(nil)
	assert.Same(t, connecting, p.Bind(testSessionKey(2)).Connection())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	connecting.ctx = ctx
	connecting.OnClose(nil, nil)
	assert.Same(t, active, p.Bind(testSessionKey(3)).Connection())
}

func TestDynamicConnectionPoolUnbindAndCleanup(t *testing.T) {
	p := newTestConnectionPool(1, 0)
	p.minConns = 0
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

func TestDynamicConnectionPoolKeepsConfiguredMinimumConnections(t *testing.T) {
	p := newTestConnectionPool(1, 0)
	p.minConns = 2
	defer p.Shutdown()
	addTestConnection(p)
	addTestConnection(p)

	p.mu.Lock()
	for _, item := range p.connections {
		item.idleSince = time.Now().Add(-time.Second)
	}
	p.mu.Unlock()
	p.cleanupIdle(time.Now())

	assert.Equal(t, 2, connectionCount(p))
}

func TestDynamicConnectionPoolStartsMinimumConnections(t *testing.T) {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		MinConnections:           2,
		MaxSessionsPerConnection: 4,
		CleanupInterval:          time.Hour,
	}, nil)
	defer p.Shutdown()
	p.newConnection = func() (*Connection, error) {
		return testConnection(), nil
	}
	p.connect = func(*Connection) error { return nil }

	p.startMinConnections()
	require.Eventually(t, func() bool {
		return connectionCount(p) == 2
	}, time.Second, time.Millisecond)
}

func TestDynamicConnectionPoolDropsClosedConnections(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()

	firstKey := testSessionKey(1)
	secondKey := testSessionKey(2)
	first := p.Bind(firstKey)
	second := p.Bind(secondKey)
	closed := first.Connection()
	require.NotNil(t, closed)
	require.Same(t, closed, second.Connection())
	replacement := addTestConnectionWithState(p, protocol.ConnectionActive)
	closed.Close()

	assert.Same(t, replacement, first.Connection())
	assert.Same(t, replacement, second.Connection())
	assert.NotContains(t, p.connections, closed)
	p.mu.Lock()
	assert.Same(t, p.connections[replacement], p.routes[firstKey])
	assert.Same(t, p.connections[replacement], p.routes[secondKey])
	p.mu.Unlock()
}

func TestDynamicConnectionPoolAllowsOnlyOnePendingDial(t *testing.T) {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		MaxSessionsPerConnection: 64,
		CleanupInterval:          time.Hour,
	}, nil)
	defer p.Shutdown()

	started := make(chan struct{}, 1)
	continueDial := make(chan struct{})
	p.newConnection = func() (*Connection, error) {
		started <- struct{}{}
		<-continueDial
		return testConnection(), nil
	}
	p.connect = func(*Connection) error { return nil }

	bindings := []*SessionBinding{p.Bind(testSessionKey(1))}
	require.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	for value := byte(2); value <= 8; value++ {
		bindings = append(bindings, p.Bind(testSessionKey(value)))
	}
	p.mu.Lock()
	assert.Equal(t, 1, p.pendingDials)
	p.mu.Unlock()

	close(continueDial)
	require.Eventually(t, func() bool {
		if connectionCount(p) != 1 {
			return false
		}
		for _, binding := range bindings {
			if binding.Connection() == nil {
				return false
			}
		}
		return true
	}, time.Second, time.Millisecond)
}

func TestDynamicConnectionPoolShutdown(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	key := testSessionKey(1)
	binding := p.Bind(key)
	conn := binding.Connection()
	require.NotNil(t, conn)

	p.Shutdown()
	p.Shutdown()

	assert.Nil(t, binding.Connection())
	assert.Equal(t, protocol.ConnectionClosed, conn.State())
	p.mu.Lock()
	assert.Empty(t, p.connections)
	assert.Empty(t, p.routes)
	assert.Empty(t, p.bindings)
	assert.Empty(t, p.queue)
	p.mu.Unlock()
}

func TestDynamicConnectionPoolSnapshots(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()

	key := testSessionKey(1)
	conn := p.Bind(key).Connection()
	snapshots := p.Snapshots()
	require.Len(t, snapshots, 1)
	assert.NotZero(t, snapshots[0].ID)
	assert.Equal(t, protocol.ConnectionActive, snapshots[0].State)
	assert.Equal(t, 1, snapshots[0].Sessions)
	assert.Zero(t, snapshots[0].IdleSince)

	p.Unbind(key)
	snapshots = p.Snapshots()
	require.Len(t, snapshots, 1)
	assert.Equal(t, conn.State(), snapshots[0].State)
	assert.Zero(t, snapshots[0].Sessions)
	assert.NotZero(t, snapshots[0].IdleSince)
}

func TestDynamicConnectionPoolIgnoresUnknownConnectionObserverEvents(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()

	unknown := testConnection()
	p.OnConnectionOpen(unknown)
	p.OnConnectionClose(unknown)

	assert.Equal(t, 1, connectionCount(p))
}

func TestNewConnectionPoolRejectsNilEngine(t *testing.T) {
	pool, err := NewConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{}, nil)
	assert.Nil(t, pool)
	assert.ErrorIs(t, err, ErrNilNBHTTPEngine)
}

func TestConnectionPoolNormalizesConfig(t *testing.T) {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		TargetSessionsPerConnection: 8,
		MaxSessionsPerConnection:    4,
	}, nil)
	defer p.Shutdown()

	assert.Equal(t, defaultMinConnections, p.minConns)
	assert.Equal(t, 4, p.targetSessions)
	assert.Equal(t, 4, p.maxSessions)
	assert.Equal(t, defaultIdleTimeout, p.idleTimeout)
	assert.Equal(t, defaultCleanupInterval, p.cleanup)
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
	addTestConnectionWithState(p, protocol.ConnectionActive)
}

func addTestConnectionWithState(p *ConnectionPool, state protocol.ConnectionState) *Connection {
	conn := testConnection()
	conn.state.Store(int32(state))
	conn.observer = p
	newConnection := p.newConnection
	p.newConnection = func() (*Connection, error) {
		return conn, nil
	}
	p.mu.Lock()
	p.pendingDials++
	gatewayPoolMetrics.pendingDials.Add(gatewayPoolMetricContext, 1)
	p.mu.Unlock()
	p.startDial()
	p.newConnection = newConnection
	return conn
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

func BenchmarkDynamicConnectionPoolBindUnbind(b *testing.B) {
	for _, connectionCount := range []int{1, 8, 64, 256, 1024} {
		b.Run(fmt.Sprintf("connections=%d", connectionCount), func(b *testing.B) {
			p := newBenchmarkConnectionPool(b.N, connectionCount)
			defer p.Shutdown()

			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				key := benchmarkSessionKey(uint64(i))
				p.Bind(key)
				p.Unbind(key)
			}
		})
	}
}

func BenchmarkDynamicConnectionPoolBindUnbindParallel(b *testing.B) {
	for _, connectionCount := range []int{8, 64, 256, 1024} {
		b.Run(fmt.Sprintf("connections=%d", connectionCount), func(b *testing.B) {
			p := newBenchmarkConnectionPool(b.N, connectionCount)
			defer p.Shutdown()

			var next atomic.Uint64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					key := benchmarkSessionKey(next.Add(1))
					p.Bind(key)
					p.Unbind(key)
				}
			})
		})
	}
}

func BenchmarkDynamicConnectionPoolStateTransition(b *testing.B) {
	for _, connectionCount := range []int{64, 1024} {
		b.Run(fmt.Sprintf("connections=%d", connectionCount), func(b *testing.B) {
			p := newBenchmarkConnectionPool(b.N, connectionCount)
			defer p.Shutdown()
			conn := p.queue[0].conn

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				conn.state.Store(int32(protocol.ConnectionConnecting))
				p.OnConnectionClose(conn)
				conn.state.Store(int32(protocol.ConnectionActive))
				p.OnConnectionOpen(conn)
			}
		})
	}
}

func BenchmarkDynamicConnectionPoolBindUnbindMixedCapacity(b *testing.B) {
	for _, connectionCount := range []int{64, 1024} {
		b.Run(fmt.Sprintf("connections=%d", connectionCount), func(b *testing.B) {
			p := newMixedBenchmarkConnectionPool(connectionCount)
			defer p.Shutdown()

			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				key := benchmarkSessionKey(uint64(i))
				p.Bind(key)
				p.Unbind(key)
			}
		})
	}
}

func BenchmarkDynamicConnectionPoolBindSameKeyParallel(b *testing.B) {
	for _, connectionCount := range []int{64, 1024} {
		b.Run(fmt.Sprintf("connections=%d", connectionCount), func(b *testing.B) {
			p := newBenchmarkConnectionPool(b.N, connectionCount)
			defer p.Shutdown()
			key := benchmarkSessionKey(1)

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					p.Bind(key)
				}
			})
		})
	}
}

func BenchmarkDynamicConnectionPoolSelect(b *testing.B) {
	for _, connectionCount := range []int{64, 1024} {
		b.Run(fmt.Sprintf("connections=%d", connectionCount), func(b *testing.B) {
			p := newMixedBenchmarkConnectionPool(connectionCount)
			defer p.Shutdown()

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				p.mu.Lock()
				item := p.selectLocked()
				p.mu.Unlock()
				if item == nil {
					b.Fatal("expected an allocatable connection")
				}
			}
		})
	}
}

func newBenchmarkConnectionPool(iterations, connectionCount int) *ConnectionPool {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		MaxSessionsPerConnection: iterations + connectionCount + 1,
		MaxConnections:           connectionCount,
		CleanupInterval:          time.Hour,
	}, nil)
	p.targetSessions = p.maxSessions
	p.newConnection = func() (*Connection, error) {
		return testConnection(), nil
	}
	p.connect = func(*Connection) error { return nil }
	for range connectionCount {
		addTestConnection(p)
	}
	return p
}

func newMixedBenchmarkConnectionPool(connectionCount int) *ConnectionPool {
	p := newConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
		MaxSessionsPerConnection: 2,
		MaxConnections:           connectionCount,
		CleanupInterval:          time.Hour,
	}, nil)
	p.targetSessions = p.maxSessions
	p.newConnection = func() (*Connection, error) {
		return testConnection(), nil
	}
	p.connect = func(*Connection) error { return nil }
	for range connectionCount {
		addTestConnection(p)
	}

	p.mu.Lock()
	for i, item := range p.queue {
		switch i % 3 {
		case 0:
			item.load = p.maxSessions
		case 1:
			item.conn.state.Store(int32(protocol.ConnectionConnecting))
		}
		item.priority = p.priorityLocked(item)
	}
	heap.Init(&p.queue)
	p.mu.Unlock()
	return p
}

func benchmarkSessionKey(value uint64) protocol.SessionKey {
	var key protocol.SessionKey
	for i := range 8 {
		key[len(key)-1-i] = byte(value >> (8 * i))
	}
	return key
}
