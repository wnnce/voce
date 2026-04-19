package realtime

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/protocol"
)

type Handler struct {
	sm *engine.SessionManager
}

func NewHandler(sm *engine.SessionManager) *Handler {
	return &Handler{sm: sm}
}

func (h *Handler) Connect(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	sessionKey, err := protocol.ParseSessionKey(sessionId)
	if err != nil {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "invalid session id")
	}
	session, ok := h.sm.LoadSession(sessionKey)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	state := session.Workflow.State()
	if state != engine.WorkflowStateRunning && state != engine.WorkflowStatePaused {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "Session is not available")
	}
	upgrader := gws.NewUpgrader(NewSocketHandler(h.sm, session), &gws.ServerOption{})
	socket, err := upgrader.Upgrade(w, request)
	if err != nil {
		return errcode.New(http.StatusInternalServerError, http.StatusInternalServerError, "Upgrade websocket failed")
	}
	go socket.ReadLoop()
	return nil
}
