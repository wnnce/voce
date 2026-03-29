package realtime

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
)

func Connect(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	manager := engine.DefaultSessionManager
	session, ok := manager.LoadSession(sessionId)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	state := session.Workflow.State()
	if state != engine.WorkflowStateRunning && state != engine.WorkflowStatePaused {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "Session is not available")
	}
	upgrader := gws.NewUpgrader(NewSocketHandler(session), &gws.ServerOption{})
	socket, err := upgrader.Upgrade(w, request)
	if err != nil {
		return errcode.New(http.StatusInternalServerError, http.StatusInternalServerError, "Upgrade websocket failed")
	}
	go socket.ReadLoop()
	return nil
}
