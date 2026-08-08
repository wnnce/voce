package handler

import (
	"net/http"

	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/machine"
)

type MachineHandler struct {
	cm *machine.ConnectionManager
}

func NewMachineHandler(cm *machine.ConnectionManager) *MachineHandler {
	return &MachineHandler{
		cm: cm,
	}
}

func (m *MachineHandler) PoolConnection(w http.ResponseWriter, r *http.Request) error {
	connection := m.cm.NewConnection()
	upgrader := gws.NewUpgrader(connection, &gws.ServerOption{})
	socket, err := upgrader.Upgrade(w, r)
	if err != nil {
		return errcode.New(http.StatusInternalServerError, http.StatusInternalServerError, "upgrade websocket failed")
	}
	go socket.ReadLoop()
	return nil
}
