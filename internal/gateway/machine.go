package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/wnnce/voce/internal/protocol"
)

// MachineState represents the current health and availability status of a backend worker.
type MachineState int32

const (
	// MachineStateActive means the machine is fully connected and ready to handle sessions.
	MachineStateActive MachineState = iota + 1
	// MachineStateSuspended means the machine control connection is lost, but sessions may still be buffered.
	MachineStateSuspended
	// MachineStateTerminated means the machine has been decommissioned and resources cleaned up.
	MachineStateTerminated
)

var ErrMachineNotActive = errors.New("machine is not active")

// Machine represents a backend worker pod that executes workflows.
// It maintains a control socket and a pool of data connections.
type Machine struct {
	ID            string
	Host          string
	Port          int
	Pool          *ConnectionPool
	socket        atomic.Pointer[websocket.Conn]
	state         atomic.Int32
	mu            sync.RWMutex
	sessions      map[protocol.SessionKey]struct{}
	lastHeartbeat atomic.Int64
}

type MachineSnapshot struct {
	ID            string                       `json:"id"`
	Address       string                       `json:"address"`
	State         MachineState                 `json:"state"`
	Sessions      int32                        `json:"sessions"`
	LastHeartbeat int64                        `json:"last_heartbeat"`
	Pool          []ConnectionPoolSlotSnapshot `json:"pool"`
}

func NewMachine(
	ctx context.Context,
	engine *nbhttp.Engine,
	id, host string, port,
	poolSize int,
	onMessage func(protocol.SessionKey, []byte),
) (*Machine, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	pool, err := NewConnectionPool(ctx, engine, id, addr, poolSize, onMessage)
	if err != nil {
		return nil, err
	}
	m := &Machine{
		ID:       id,
		Host:     host,
		Port:     port,
		Pool:     pool,
		sessions: make(map[protocol.SessionKey]struct{}),
	}
	m.state.Store(int32(MachineStateSuspended))
	return m, nil
}

func (m *Machine) State() MachineState {
	return MachineState(m.state.Load())
}

func (m *Machine) Sessions() int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int32(len(m.sessions))
}

func (m *Machine) AddSession(key protocol.SessionKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[key] = struct{}{}
}

func (m *Machine) RemoveSession(key protocol.SessionKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, key)
}

func (m *Machine) RangeSessions(fn func(key protocol.SessionKey) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k := range m.sessions {
		if !fn(k) {
			break
		}
	}
}

func (m *Machine) Address() string {
	return fmt.Sprintf("%s:%d", m.Host, m.Port)
}

func (m *Machine) OnPong(_ *websocket.Conn, _ string) {
	m.lastHeartbeat.Store(time.Now().UnixMilli())
}

func (m *Machine) OnMessage(_ *websocket.Conn, _ websocket.MessageType, _ []byte) {
	slog.Info("Received machine message")
}

func (m *Machine) OnOpen(socket *websocket.Conn) {
	slog.Info("machine control connection opened", "id", m.ID, "addr", m.Address())
	m.lastHeartbeat.Store(time.Now().UnixMilli())
	m.socket.Store(socket)
	m.state.Store(int32(MachineStateActive))
}

func (m *Machine) OnClose(_ *websocket.Conn, err error) {
	slog.Warn("machine control connection closed", "id", m.ID, "addr", m.Address(), "error", err)
	m.state.Store(int32(MachineStateSuspended))
	m.socket.Store(nil)
}

func (m *Machine) Heartbeat() error {
	socket := m.socket.Load()
	if m.State() != MachineStateActive || socket == nil {
		return ErrMachineNotActive
	}
	return socket.WriteMessage(websocket.PingMessage, nil)
}

func (m *Machine) Snapshot() MachineSnapshot {
	return MachineSnapshot{
		ID:            m.ID,
		Address:       m.Address(),
		State:         m.State(),
		Sessions:      m.Sessions(),
		LastHeartbeat: m.lastHeartbeat.Load(),
		Pool:          m.Pool.Snapshots(),
	}
}
