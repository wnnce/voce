package gateway

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wnnce/voce/internal/protocol"
)

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
	for _, connectionCount := range []int{8, 64, 256} {
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

func newBenchmarkConnectionPool(iterations, connectionCount int) *dynamicConnectionPool {
	p := newDynamicConnectionPool(context.Background(), nil, "machine", "127.0.0.1:7001", ConnectionPoolConfig{
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

func benchmarkSessionKey(value uint64) protocol.SessionKey {
	var key protocol.SessionKey
	for i := range 8 {
		key[len(key)-1-i] = byte(value >> (8 * i))
	}
	return key
}
