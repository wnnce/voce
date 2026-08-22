package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/protocol"
)

func TestNewMachineRejectsNilEngine(t *testing.T) {
	machine, err := NewMachine(context.Background(), nil, "machine", "127.0.0.1", 7001, config.GatewayServerConfig{}, nil)
	assert.Nil(t, machine)
	assert.ErrorIs(t, err, ErrNilNBHTTPEngine)
}

func TestMachineLifecycleAndSnapshot(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()
	machine := &Machine{
		ID:       "machine",
		Host:     "127.0.0.1",
		Port:     7001,
		Pool:     p,
		sessions: make(map[protocol.SessionKey]struct{}),
	}
	machine.state.Store(int32(MachineStateSuspended))
	key := testSessionKey(1)
	machine.AddSession(key)
	assert.Equal(t, int32(1), machine.Sessions())

	var ranged []protocol.SessionKey
	machine.RangeSessions(func(key protocol.SessionKey) bool {
		ranged = append(ranged, key)
		return true
	})
	assert.Equal(t, []protocol.SessionKey{key}, ranged)

	before := machine.lastHeartbeat.Load()
	machine.OnPong(nil, "")
	assert.Greater(t, machine.lastHeartbeat.Load(), before)
	machine.OnOpen(nil)
	assert.Equal(t, MachineStateActive, machine.State())
	require.ErrorIs(t, machine.Heartbeat(), ErrMachineNotActive)

	snapshot := machine.Snapshot()
	assert.Equal(t, "127.0.0.1:7001", snapshot.Address)
	assert.Equal(t, MachineStateActive, snapshot.State)
	assert.Equal(t, int32(1), snapshot.Sessions)
	assert.NotEmpty(t, snapshot.Pool)

	machine.OnClose(nil, nil)
	assert.Equal(t, MachineStateSuspended, machine.State())
	machine.RemoveSession(key)
	assert.Zero(t, machine.Sessions())
}

func TestMachineManagerSelectionAndAcquireExisting(t *testing.T) {
	pools := []*ConnectionPool{newTestConnectionPool(4, 0), newTestConnectionPool(4, 0), newTestConnectionPool(4, 0)}
	defer func() {
		for _, p := range pools {
			p.Shutdown()
		}
	}()
	activeFew := testMachine("few", pools[0], MachineStateActive, 1)
	activeMany := testMachine("many", pools[1], MachineStateActive, 2)
	suspended := testMachine("suspended", pools[2], MachineStateSuspended, 0)
	manager := &MachineManager{items: map[string]*Machine{
		activeFew.ID:  activeFew,
		activeMany.ID: activeMany,
		suspended.ID:  suspended,
	}}

	assert.Same(t, activeFew, manager.LeastSessions())
	random := manager.Random()
	assert.Contains(t, []*Machine{activeFew, activeMany}, random)

	got, err := manager.AcquireMachine(activeFew.ID, activeFew.Host, activeFew.Port)
	require.NoError(t, err)
	assert.Same(t, activeFew, got)
	_, err = manager.AcquireMachine(activeFew.ID, "127.0.0.2", activeFew.Port)
	require.ErrorIs(t, err, ErrMachineIDConflict)
	activeFew.state.Store(int32(MachineStateTerminated))
	_, err = manager.AcquireMachine(activeFew.ID, activeFew.Host, activeFew.Port)
	assert.ErrorIs(t, err, ErrMachineAlreadyActive)
}

func TestMachineManagerCleansExpiredMachine(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	key := testSessionKey(1)
	machine := testMachine("machine", p, MachineStateActive, 0)
	machine.AddSession(key)
	machine.lastHeartbeat.Store(time.Now().Add(-time.Second).UnixMilli())
	sm := newTestSessionManager()
	sm.Store(NewSession(key, p.Bind(key), machine))
	manager := &MachineManager{items: map[string]*Machine{machine.ID: machine}, sm: sm}

	manager.cleanupMachines(time.Millisecond)
	_, exists := manager.LoadMachine(machine.ID)
	assert.False(t, exists)
	_, exists = sm.Load(key)
	assert.False(t, exists)
	assert.Equal(t, MachineStateTerminated, machine.State())
	assert.True(t, p.closed.Load())
}

func testMachine(id string, pool *ConnectionPool, state MachineState, sessionCount int) *Machine {
	machine := &Machine{
		ID:       id,
		Host:     "127.0.0.1",
		Port:     7001,
		Pool:     pool,
		sessions: make(map[protocol.SessionKey]struct{}, sessionCount),
	}
	machine.state.Store(int32(state))
	for i := range sessionCount {
		machine.sessions[testSessionKey(byte(i+1))] = struct{}{}
	}
	return machine
}
