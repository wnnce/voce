package gateway

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/protocol"
)

var (
	ErrMachineAlreadyActive = errcode.New(http.StatusConflict, http.StatusConflict, "machine already registered and active")
	ErrMachineIDConflict    = errcode.New(http.StatusConflict, http.StatusConflict, "machine ID conflict: host or port mismatch")
)

const defaultMachinePort = 7001

// MachineManager handles the lifecycle and health of backend workers (machines).
// It maintains a registry of active machines and performs periodic heartbeats and cleanup.
type MachineManager struct {
	ctx    context.Context
	config config.GatewayServerConfig
	mu     sync.RWMutex
	items  map[string]*Machine
	sm     *SessionManager
	engine *nbhttp.Engine
}

// NewMachineManager initializes a new MachineManager and starts its background routines.
func NewMachineManager(ctx context.Context, cfg config.GatewayServerConfig, sm *SessionManager, engine *nbhttp.Engine) *MachineManager {
	m := &MachineManager{
		ctx:    ctx,
		config: cfg,
		sm:     sm,
		engine: engine,
		items:  make(map[string]*Machine),
	}
	slog.Info("machine manager started", "heartbeat", cfg.HeartbeatInterval, "cleanup", cfg.CleanupInterval)
	go m.run(ctx)
	return m
}

func (m *MachineManager) run(ctx context.Context) {
	heartbeatTicker := time.NewTicker(m.config.HeartbeatInterval)
	cleanupTicker := time.NewTicker(m.config.CleanupInterval)
	defer func() {
		heartbeatTicker.Stop()
		cleanupTicker.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			m.sendHeartbeats()
		case <-cleanupTicker.C:
			m.cleanupMachines(m.config.SuspendTimeout)
		}
	}
}

func (m *MachineManager) RangeMachines(fn func(id string, machine *Machine) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, machine := range m.items {
		if !fn(id, machine) {
			break
		}
	}
}

func (m *MachineManager) LoadMachine(id string) (*Machine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	machine, ok := m.items[id]
	return machine, ok
}

func (m *MachineManager) StoreMachine(id string, machine *Machine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[id] = machine
}

func (m *MachineManager) DeleteMachine(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)
}

func (m *MachineManager) sendHeartbeats() {
	m.RangeMachines(func(_ string, machine *Machine) bool {
		if err := machine.Heartbeat(); err != nil {
			slog.Error("failed to send heartbeat to machine", "error", err, "id", machine.ID, "addr", machine.Address())
		}
		return true
	})
}

func (m *MachineManager) cleanupMachines(timeout time.Duration) {
	threshold := time.Now().Add(-timeout).UnixMilli()
	var expired []string

	m.RangeMachines(func(id string, machine *Machine) bool {
		last := machine.lastHeartbeat.Load()
		if last > 0 && last < threshold {
			expired = append(expired, id)
		}
		return true
	})

	for _, id := range expired {
		machine, ok := m.LoadMachine(id)
		if !ok {
			continue
		}
		slog.Warn("machine heartbeat timeout, cleaning up", "id", id, "addr", machine.Address())
		machine.state.Store(int32(MachineStateTerminated))

		var sessionKeys []protocol.SessionKey
		machine.RangeSessions(func(key protocol.SessionKey) bool {
			sessionKeys = append(sessionKeys, key)
			return true
		})

		for _, key := range sessionKeys {
			m.sm.Delete(key)
		}

		machine.Pool.Shutdown()
		m.DeleteMachine(id)
	}
}

func (m *MachineManager) LeastSessions() *Machine {
	var selected *Machine
	var minSessions int32 = math.MaxInt32
	m.RangeMachines(func(_ string, machine *Machine) bool {
		if machine.State() != MachineStateActive {
			return true
		}
		if s := machine.Sessions(); s < minSessions {
			minSessions = s
			selected = machine
		}
		return true
	})
	return selected
}

func (m *MachineManager) Random() *Machine {
	var selected *Machine
	m.RangeMachines(func(_ string, machine *Machine) bool {
		if machine.State() == MachineStateActive {
			selected = machine
			return false
		}
		return true
	})
	return selected
}

func (m *MachineManager) AcquireMachine(id, host string, port int) (*Machine, error) {
	// First check: read lock
	m.mu.RLock()
	existing, ok := m.items[id]
	m.mu.RUnlock()
	if ok {
		if err := m.checkMachine(existing, host, port); err != nil {
			return nil, err
		}
		return existing, nil
	}

	// Double check: write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok = m.items[id]; ok {
		if err := m.checkMachine(existing, host, port); err != nil {
			return nil, err
		}
		return existing, nil
	}

	machine, err := NewMachine(m.ctx, m.engine, id, host, port, m.config.PoolSize, m.sm.DispatchMessage)
	if err != nil {
		return nil, errcode.NewInternal(err.Error())
	}

	m.items[id] = machine
	slog.Info("machine acquired", "id", id, "host", host, "port", port)
	return machine, nil
}

func (m *MachineManager) checkMachine(machine *Machine, host string, port int) error {
	if machine.State() != MachineStateSuspended && machine.State() != MachineStateActive {
		return ErrMachineAlreadyActive
	}
	if machine.Host != host || machine.Port != port {
		return ErrMachineIDConflict
	}
	return nil
}
