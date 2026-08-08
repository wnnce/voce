package handler

import (
	"log/slog"
	"net/http"
	"strconv"

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
	query := r.URL.Query()
	index, err := strconv.Atoi(query.Get("slot"))
	slog.Info("gateway request pool connection", "slot", index)
	if err != nil {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "invalid slot index")
	}
	connection := m.cm.NewConnection()
	upgrader := gws.NewUpgrader(connection, &gws.ServerOption{})
	socket, err := upgrader.Upgrade(w, r)
	if err != nil {
		return errcode.New(http.StatusInternalServerError, http.StatusInternalServerError, "upgrade websocket failed")
	}
	go socket.ReadLoop()
	return nil
}
