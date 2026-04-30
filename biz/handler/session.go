package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/internal/machine"
	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type SessionHandler struct {
	sm *engine.SessionManager
	cm *machine.ConnectionManager
}

func NewGatewaySessionHandler(sm *engine.SessionManager, cm *machine.ConnectionManager) *SessionHandler {
	return &SessionHandler{
		sm: sm,
		cm: cm,
	}
}

func NewStandaloneSessionHandler(sm *engine.SessionManager) *SessionHandler {
	return &SessionHandler{sm: sm}
}

type SessionMetadata struct {
	Language   string `json:"language,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	DeviceCode string `json:"device_code,omitempty"`
	IP         string `json:"ip,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
}

type CreateSessionRequest struct {
	Name       string                     `json:"name"`
	Metadata   *SessionMetadata           `json:"metadata,omitempty"`
	Properties map[string]json.RawMessage `json:"properties,omitempty"`
}

type HealthResponse struct {
	ActiveTime int64     `json:"active_time"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *SessionHandler) CreateWorkflowSession(w http.ResponseWriter, request *http.Request) error {
	var payload CreateSessionRequest
	if err := httpx.BindJSON(request, &payload); err != nil {
		return err
	}
	sessionKey := protocol.NewSessionKey()
	sessionId := sessionKey.String()
	sessionCtx := context.WithValue(h.sm.Context(), metadata.ContextTraceKey, sessionId)
	session, err := h.sm.CreateSession(sessionCtx, sessionKey, payload.Name, payload.Properties)
	if err != nil {
		return err
	}
	if h.cm != nil {
		go h.sessionWriteLoop(session)
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(map[string]any{
		"session_id": sessionId,
	}))
}

func (h *SessionHandler) WorkflowSessionHealth(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	sessionKey, err := protocol.ParseSessionKey(sessionId)
	if err != nil {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "invalid session id")
	}
	session, ok := h.sm.LoadSession(sessionKey)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	var state string
	switch session.Workflow.State() {
	case engine.WorkflowStatePending:
		state = "pending"
	case engine.WorkflowStateStarting:
		state = "starting"
	case engine.WorkflowStateRunning:
		state = "running"
	case engine.WorkflowStatePaused:
		state = "paused"
	case engine.WorkflowStateStopped:
		state = "stopped"
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(HealthResponse{
		ActiveTime: session.LastActive.Load(),
		State:      state,
		CreatedAt:  session.CreatedAt,
	}))
}

func (h *SessionHandler) RenewWorkflowSession(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	sessionKey, err := protocol.ParseSessionKey(sessionId)
	if err != nil {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "invalid session id")
	}
	session, ok := h.sm.LoadSession(sessionKey)
	if !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	session.UpdateActivity()
	return httpx.JSON(w, http.StatusOK, result.Success())
}

func (h *SessionHandler) DeleteWorkflowSession(w http.ResponseWriter, request *http.Request) error {
	sessionId := chi.URLParam(request, "id")
	sessionKey, err := protocol.ParseSessionKey(sessionId)
	if err != nil {
		return errcode.New(http.StatusBadRequest, http.StatusBadRequest, "invalid session id")
	}
	if _, ok := h.sm.LoadSession(sessionKey); !ok {
		return errcode.New(http.StatusNotFound, http.StatusNotFound, "Session not found")
	}
	h.sm.RemoveSession(sessionKey)
	return httpx.JSON(w, http.StatusOK, result.Success())
}

func (h *SessionHandler) sessionWriteLoop(session *engine.Session) {
	conn := h.cm.Select(session.Key)
	if conn == nil {
		slog.ErrorContext(session.Workflow.Context(), "failed to select connection", "session_id", session.Key.String())
		return
	}
	ctx := session.Workflow.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok := <-session.Workflow.Output():
			if !ok {
				return
			}
			if err := conn.Write(session.Key, packet); err != nil {
				slog.Error("failed to write packet to connection", "error", err, "session_id", session.Key.String())
			}
			session.UpdateActivity()
			protocol.ReleasePacket(packet)
		}
	}
}
