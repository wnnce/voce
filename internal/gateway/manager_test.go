package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineManagerCheckMachine(t *testing.T) {
	machine := &Machine{Host: "127.0.0.1", Port: 7001}
	manager := &MachineManager{}

	for _, state := range []MachineState{MachineStateSuspended, MachineStateActive} {
		machine.state.Store(int32(state))
		require.NoError(t, manager.checkMachine(machine, machine.Host, machine.Port))
	}
	machine.state.Store(int32(MachineStateTerminated))
	assert.ErrorIs(t, manager.checkMachine(machine, machine.Host, machine.Port), ErrMachineAlreadyActive)
}
