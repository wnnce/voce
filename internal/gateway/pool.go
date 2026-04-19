package gateway

import (
	"context"
	"sync/atomic"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/wnnce/voce/internal/protocol"
)

const (
	offset32 = 2166136261
	prime32  = 16777619
)

type ConnectionPool struct {
	slots  []*Connection
	closed atomic.Bool
}

func NewConnectionPool(
	ctx context.Context,
	engine *nbhttp.Engine,
	machineID, address string,
	size int,
	dispatcher MessageDispatcher,
) (*ConnectionPool, error) {
	p := &ConnectionPool{
		slots: make([]*Connection, size),
	}

	for i := 0; i < size; i++ {
		conn, err := NewConnection(ctx, engine, machineID, address, i, dispatcher)
		if err != nil {
			p.Shutdown()
			return nil, err
		}
		p.slots[i] = conn
	}
	return p, nil
}

// Select 获取该 SessionKey 对应的固定连接
func (p *ConnectionPool) Select(key protocol.SessionKey) *Connection {
	if p.closed.Load() || len(p.slots) == 0 {
		return nil
	}

	hash := uint32(offset32)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}

	idx := hash % uint32(len(p.slots))
	return p.slots[idx]
}

// Shutdown 关闭池中所有连接
func (p *ConnectionPool) Shutdown() {
	if p.closed.Swap(true) {
		return
	}
	for _, conn := range p.slots {
		if conn != nil {
			conn.Close()
		}
	}
}
